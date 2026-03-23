package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/auth"
	cryptopkg "harborshield/backend/internal/crypto"
)

type TopologyService struct {
	db           *pgxpool.Pool
	client       *http.Client
	store        *DistributedStore
	replicaCount int
	sealer       *cryptopkg.Sealer
}

type Node struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Endpoint        string         `json:"endpoint"`
	BackendType     string         `json:"backendType"`
	Zone            string         `json:"zone"`
	Status          string         `json:"status"`
	OperatorState   string         `json:"operatorState"`
	CapacityBytes   int64          `json:"capacityBytes"`
	UsedBytes       int64          `json:"usedBytes"`
	Metadata        map[string]any `json:"metadata"`
	LastHeartbeatAt string         `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
}

type Placement struct {
	ID             string         `json:"id"`
	ObjectID       string         `json:"objectId"`
	BucketID       string         `json:"bucketId"`
	ObjectKey      string         `json:"objectKey"`
	VersionID      string         `json:"versionId"`
	ReplicaIndex   int            `json:"replicaIndex"`
	ChunkOrdinal   int            `json:"chunkOrdinal"`
	StorageNodeID  string         `json:"storageNodeId,omitempty"`
	NodeName       string         `json:"nodeName,omitempty"`
	Locator        string         `json:"locator"`
	ChecksumSHA256 string         `json:"checksumSha256"`
	State          string         `json:"state"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

type PlacementFilter struct {
	BucketID  string
	Key       string
	VersionID string
	Limit     int
}

type JoinToken struct {
	ID               string `json:"id"`
	Description      string `json:"description"`
	IntendedName     string `json:"intendedName"`
	IntendedEndpoint string `json:"intendedEndpoint"`
	Zone             string `json:"zone"`
	ExpiresAt        string `json:"expiresAt"`
	UsedAt           string `json:"usedAt,omitempty"`
	CreatedAt        string `json:"createdAt"`
}

type IssuedJoinToken struct {
	JoinToken
	Token string `json:"token"`
}

type JoinEnrollment struct {
	Node
	RPCToken string `json:"rpcToken"`
}

var ErrInvalidOperatorState = errors.New("operatorState must be active, draining, or maintenance")
var ErrJoinTokenInvalid = errors.New("invalid or expired storage join token")
var ErrJoinTokenUsed = errors.New("storage join token has already been used")
var ErrNodeTLSIdentityUnavailable = errors.New("node TLS identity is unavailable for re-pin")

func NewTopologyService(db *pgxpool.Pool, store *DistributedStore, replicaCount int, base64Key string) *TopologyService {
	sealer, _ := cryptopkg.NewSealer(base64Key)
	client := &http.Client{Timeout: 10 * time.Second}
	if store != nil && store.client != nil {
		client = store.client
	}
	return &TopologyService{
		db:           db,
		client:       client,
		store:        store,
		replicaCount: replicaCount,
		sealer:       sealer,
	}
}

func (s *TopologyService) IssueJoinToken(ctx context.Context, description, intendedName, intendedEndpoint, zone string, expiresIn time.Duration) (IssuedJoinToken, error) {
	if expiresIn <= 0 {
		expiresIn = 15 * time.Minute
	}
	rawToken, err := randomJoinToken()
	if err != nil {
		return IssuedJoinToken{}, err
	}
	expiresAt := time.Now().UTC().Add(expiresIn)
	var item IssuedJoinToken
	item.Token = rawToken
	item.Description = strings.TrimSpace(description)
	item.IntendedName = strings.TrimSpace(intendedName)
	item.IntendedEndpoint = strings.TrimSpace(intendedEndpoint)
	item.Zone = strings.TrimSpace(zone)
	item.ExpiresAt = expiresAt.Format(time.RFC3339)
	if err := s.db.QueryRow(ctx, `
		INSERT INTO storage_join_tokens (token_hash, description, intended_name, intended_endpoint, zone, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, created_at::text
	`, hashJoinToken(rawToken), item.Description, item.IntendedName, item.IntendedEndpoint, item.Zone, expiresAt).Scan(&item.ID, &item.CreatedAt); err != nil {
		return IssuedJoinToken{}, err
	}
	return item, nil
}

func (s *TopologyService) JoinNode(ctx context.Context, rawToken, name, endpoint, zone string) (JoinEnrollment, error) {
	name = strings.TrimSpace(name)
	endpoint = strings.TrimSpace(endpoint)
	zone = strings.TrimSpace(zone)
	if name == "" || endpoint == "" {
		return JoinEnrollment{}, errors.New("name and endpoint are required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return JoinEnrollment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tokenID string
	var intendedName string
	var intendedEndpoint string
	var intendedZone string
	var usedAt string
	err = tx.QueryRow(ctx, `
		SELECT id::text, COALESCE(intended_name, ''), COALESCE(intended_endpoint, ''), COALESCE(zone, ''), COALESCE(used_at::text, '')
		FROM storage_join_tokens
		WHERE token_hash = $1
		  AND expires_at > NOW()
	`, hashJoinToken(rawToken)).Scan(&tokenID, &intendedName, &intendedEndpoint, &intendedZone, &usedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JoinEnrollment{}, ErrJoinTokenInvalid
		}
		return JoinEnrollment{}, err
	}
	if usedAt != "" {
		return JoinEnrollment{}, ErrJoinTokenUsed
	}
	if intendedName != "" && intendedName != name {
		return JoinEnrollment{}, fmt.Errorf("join token is bound to node name %s", intendedName)
	}
	if intendedEndpoint != "" && intendedEndpoint != endpoint {
		return JoinEnrollment{}, fmt.Errorf("join token is bound to endpoint %s", intendedEndpoint)
	}
	if intendedZone != "" && intendedZone != zone {
		return JoinEnrollment{}, fmt.Errorf("join token is bound to zone %s", intendedZone)
	}
	rpcToken, rpcHash, rpcCiphertext, err := s.generateRPCCredential()
	if err != nil {
		return JoinEnrollment{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO storage_nodes (name, endpoint, backend_type, zone, status, operator_state, metadata, rpc_token_hash, rpc_token_ciphertext, updated_at)
		VALUES ($1, $2, 'distributed', $3, 'configured', 'maintenance', '{"joined":true}'::jsonb, $4, $5, NOW())
		ON CONFLICT (endpoint)
		DO UPDATE SET name = EXCLUDED.name, zone = EXCLUDED.zone, metadata = '{"joined":true}'::jsonb, rpc_token_hash = EXCLUDED.rpc_token_hash, rpc_token_ciphertext = EXCLUDED.rpc_token_ciphertext, updated_at = NOW()
	`, name, endpoint, zone, rpcHash, rpcCiphertext); err != nil {
		return JoinEnrollment{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE storage_join_tokens
		SET used_at = NOW(), used_by_name = $2, used_by_endpoint = $3
		WHERE id = $1::uuid
	`, tokenID, name, endpoint); err != nil {
		return JoinEnrollment{}, err
	}

	var item JoinEnrollment
	var rawMetadata []byte
	if err := tx.QueryRow(ctx, `
		SELECT id::text, name, endpoint, backend_type, zone, status, operator_state, capacity_bytes, used_bytes, metadata, COALESCE(last_heartbeat_at::text, ''), created_at::text, updated_at::text
		FROM storage_nodes
		WHERE endpoint = $1
	`, endpoint).Scan(&item.ID, &item.Name, &item.Endpoint, &item.BackendType, &item.Zone, &item.Status, &item.OperatorState, &item.CapacityBytes, &item.UsedBytes, &rawMetadata, &item.LastHeartbeatAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return JoinEnrollment{}, err
	}
	if err := json.Unmarshal(rawMetadata, &item.Metadata); err != nil || len(item.Metadata) == 0 {
		item.Metadata = nil
	}
	if err := tx.Commit(ctx); err != nil {
		return JoinEnrollment{}, err
	}
	item.RPCToken = rpcToken
	if s.store != nil {
		s.store.SetEndpointToken(endpoint, rpcToken)
	}
	return item, nil
}

func (s *TopologyService) RegisterNode(ctx context.Context, name, endpoint, zone, operatorState string) (Node, error) {
	name = strings.TrimSpace(name)
	endpoint = strings.TrimSpace(endpoint)
	zone = strings.TrimSpace(zone)
	operatorState = strings.ToLower(strings.TrimSpace(operatorState))
	if name == "" || endpoint == "" {
		return Node{}, errors.New("name and endpoint are required")
	}
	if operatorState == "" {
		operatorState = "maintenance"
	}
	if operatorState != "active" && operatorState != "draining" && operatorState != "maintenance" {
		return Node{}, ErrInvalidOperatorState
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO storage_nodes (name, endpoint, backend_type, zone, status, operator_state, metadata, updated_at)
		VALUES ($1, $2, 'distributed', $3, 'configured', $4, '{"configured":false}'::jsonb, NOW())
	`, name, endpoint, zone, operatorState)
	if err != nil {
		return Node{}, err
	}

	nodes, err := s.ListNodes(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, node := range nodes {
		if node.Endpoint == endpoint {
			return node, nil
		}
	}
	return Node{}, pgx.ErrNoRows
}

type nodeHealthPayload struct {
	Status        string `json:"status"`
	Service       string `json:"service"`
	Name          string `json:"name"`
	CapacityBytes int64  `json:"capacityBytes"`
	UsedBytes     int64  `json:"usedBytes"`
	FileCount     int64  `json:"fileCount"`
}

type nodeProbeResult struct {
	Health               nodeHealthPayload
	TLSFingerprintSHA256 string
	TLSCommonName        string
}

func (s *TopologyService) SyncConfiguredNodes(ctx context.Context, endpoints []string) error {
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		name := nodeNameForEndpoint(endpoint)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO storage_nodes (name, endpoint, backend_type, status, metadata, updated_at)
			VALUES ($1, $2, 'distributed', 'configured', '{"configured":true}'::jsonb, NOW())
			ON CONFLICT (endpoint)
			DO UPDATE SET name = EXCLUDED.name, status = 'configured', metadata = '{"configured":true}'::jsonb, updated_at = NOW()
		`, name, endpoint); err != nil {
			return err
		}
	}
	return nil
}

func (s *TopologyService) RefreshConfiguredNodes(ctx context.Context) (int64, error) {
	if err := s.syncStoreTokens(ctx); err != nil {
		return 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT endpoint, metadata FROM storage_nodes ORDER BY endpoint ASC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type nodeRefreshRow struct {
		Endpoint string
		Metadata map[string]any
	}
	var items []nodeRefreshRow
	for rows.Next() {
		var endpoint string
		var rawMetadata []byte
		if err := rows.Scan(&endpoint, &rawMetadata); err != nil {
			return 0, err
		}
		var metadata map[string]any
		if len(rawMetadata) > 0 {
			_ = json.Unmarshal(rawMetadata, &metadata)
		}
		items = append(items, nodeRefreshRow{Endpoint: endpoint, Metadata: metadata})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var refreshed int64
	for _, item := range items {
		endpoint := item.Endpoint
		status := "offline"
		var capacityBytes int64
		var usedBytes int64
		metadataMap := map[string]any{"reachable": false}
		for key, value := range item.Metadata {
			metadataMap[key] = value
		}
		var metadata string
		var heartbeat any

		probe, err := s.fetchNodeHealth(ctx, endpoint)
		if err == nil {
			status = "healthy"
			capacityBytes = probe.Health.CapacityBytes
			usedBytes = probe.Health.UsedBytes
			heartbeat = time.Now().UTC()
			metadataMap["reachable"] = true
			metadataMap["service"] = probe.Health.Service
			metadataMap["fileCount"] = probe.Health.FileCount
			if probe.TLSFingerprintSHA256 != "" {
				expectedFingerprint, _ := metadataMap["tlsExpectedFingerprintSha256"].(string)
				if strings.TrimSpace(expectedFingerprint) == "" {
					metadataMap["tlsExpectedFingerprintSha256"] = probe.TLSFingerprintSHA256
					metadataMap["tlsIdentityStatus"] = "pinned"
				} else if strings.EqualFold(expectedFingerprint, probe.TLSFingerprintSHA256) {
					metadataMap["tlsIdentityStatus"] = "verified"
				} else {
					status = "tls-mismatch"
					metadataMap["tlsIdentityStatus"] = "mismatch"
				}
				metadataMap["tlsObservedFingerprintSha256"] = probe.TLSFingerprintSHA256
			}
			if probe.TLSCommonName != "" {
				metadataMap["tlsCommonName"] = probe.TLSCommonName
			}
		}
		rawMetadata, marshalErr := json.Marshal(metadataMap)
		if marshalErr == nil {
			metadata = string(rawMetadata)
		} else {
			metadata = `{"reachable":false}`
		}

		if _, err := s.db.Exec(ctx, `
			UPDATE storage_nodes
			SET status = $2,
			    capacity_bytes = $3,
			    used_bytes = $4,
			    metadata = $5::jsonb,
			    last_heartbeat_at = $6,
			    updated_at = NOW()
			WHERE endpoint = $1
		`, endpoint, status, capacityBytes, usedBytes, metadata, heartbeat); err != nil {
			return refreshed, err
		}
		refreshed++
	}
	return refreshed, nil
}

func (s *TopologyService) RefreshPlacementHealth(ctx context.Context) (int64, error) {
	targetReplicas, err := s.targetReplicaCount(ctx)
	if err != nil {
		return 0, err
	}
	if targetReplicas == 0 {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT
			p.id::text,
			p.locator,
			COALESCE(n.endpoint, ''),
			COALESCE(n.status, 'offline')
		FROM object_placements p
		LEFT JOIN storage_nodes n ON n.id = p.storage_node_id
		ORDER BY p.created_at ASC
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var refreshed int64
	for rows.Next() {
		var placementID string
		var locator string
		var endpoint string
		var nodeStatus string
		if err := rows.Scan(&placementID, &locator, &endpoint, &nodeStatus); err != nil {
			return refreshed, err
		}
		state := "degraded"
		if nodeStatus == "healthy" && endpoint != "" {
			exists, err := s.replicaExists(ctx, endpoint, locator)
			if err == nil && exists {
				state = "stored"
			}
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE object_placements
			SET state = $2, updated_at = NOW()
			WHERE id = $1::uuid AND state <> $2
		`, placementID, state); err != nil {
			return refreshed, err
		}
		refreshed++
	}
	if err := rows.Err(); err != nil {
		return refreshed, err
	}

	objectRows, err := s.db.Query(ctx, `
		SELECT
			p.object_id::text,
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE p.state = 'stored')::bigint
		FROM object_placements p
		GROUP BY p.object_id
	`)
	if err != nil {
		return refreshed, err
	}
	defer objectRows.Close()

	for objectRows.Next() {
		var objectID string
		var placementCount int64
		var healthyCount int64
		if err := objectRows.Scan(&objectID, &placementCount, &healthyCount); err != nil {
			return refreshed, err
		}
		state := "stored"
		if placementCount < targetReplicas || healthyCount < targetReplicas {
			state = "degraded"
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE object_placements
			SET state = $2, updated_at = NOW()
			WHERE object_id = $1::uuid AND state <> $2
		`, objectID, state); err != nil {
			return refreshed, err
		}
	}
	return refreshed, objectRows.Err()
}

func (s *TopologyService) RecordPlacements(ctx context.Context, objectID, locator string, endpoints []string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM object_placements WHERE object_id = $1::uuid`, objectID); err != nil {
		return err
	}

	for index, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO object_placements (object_id, replica_index, chunk_ordinal, storage_node_id, locator, state, metadata, checksum_sha256, updated_at)
			VALUES (
				$1::uuid,
				$2,
				0,
				(SELECT id FROM storage_nodes WHERE endpoint = $3 LIMIT 1),
				$4,
				'stored',
				jsonb_build_object('endpoint', $3),
				'',
				NOW()
			)
		`, objectID, index, endpoint, locator); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *TopologyService) RepairDegradedPlacements(ctx context.Context) (int64, error) {
	if s.store == nil {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT
			p.id::text,
			p.locator,
			COALESCE(n.endpoint, ''),
			p.state
		FROM object_placements p
		LEFT JOIN storage_nodes n ON n.id = p.storage_node_id
		WHERE p.state = 'degraded'
		ORDER BY p.updated_at ASC, p.created_at ASC
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type placementRepair struct {
		id       string
		locator  string
		endpoint string
	}
	grouped := map[string][]placementRepair{}
	order := make([]string, 0)
	for rows.Next() {
		var placementID string
		var locator string
		var endpoint string
		var state string
		if err := rows.Scan(&placementID, &locator, &endpoint, &state); err != nil {
			return 0, err
		}
		if _, ok := grouped[locator]; !ok {
			order = append(order, locator)
		}
		grouped[locator] = append(grouped[locator], placementRepair{id: placementID, locator: locator, endpoint: endpoint})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var repaired int64
	for _, locator := range order {
		placements := grouped[locator]
		sourceEndpoint := ""
		for _, placement := range placements {
			if placement.endpoint == "" {
				continue
			}
			exists, err := s.replicaExists(ctx, placement.endpoint, placement.locator)
			if err != nil {
				continue
			}
			if exists {
				sourceEndpoint = placement.endpoint
				break
			}
		}
		if sourceEndpoint == "" {
			continue
		}

		for _, placement := range placements {
			if placement.endpoint == "" || placement.endpoint == sourceEndpoint {
				continue
			}
			exists, err := s.replicaExists(ctx, placement.endpoint, placement.locator)
			if err == nil && exists {
				continue
			}
			if err := s.store.CopyBetweenEndpoints(ctx, sourceEndpoint, placement.endpoint, placement.locator); err != nil {
				continue
			}
			if _, err := s.db.Exec(ctx, `
				UPDATE object_placements
				SET state = 'stored', updated_at = NOW()
				WHERE id = $1::uuid
			`, placement.id); err != nil {
				return repaired, err
			}
			repaired++
		}
	}

	return repaired, nil
}

func (s *TopologyService) RebalancePlacements(ctx context.Context) (int64, error) {
	if s.store == nil {
		return 0, nil
	}

	targetEndpoints, err := s.activeTargetEndpoints(ctx)
	if err != nil {
		return 0, err
	}
	if len(targetEndpoints) == 0 {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT
			p.id::text,
			p.object_id::text,
			p.locator,
			p.replica_index,
			COALESCE(n.id::text, ''),
			COALESCE(n.endpoint, '')
		FROM object_placements p
		LEFT JOIN storage_nodes n ON n.id = p.storage_node_id
		ORDER BY p.object_id ASC, p.replica_index ASC
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type placementRecord struct {
		id           string
		objectID     string
		locator      string
		replicaIndex int
		nodeID       string
		endpoint     string
	}
	grouped := map[string][]placementRecord{}
	order := make([]string, 0)
	for rows.Next() {
		var record placementRecord
		if err := rows.Scan(&record.id, &record.objectID, &record.locator, &record.replicaIndex, &record.nodeID, &record.endpoint); err != nil {
			return 0, err
		}
		if _, ok := grouped[record.objectID]; !ok {
			order = append(order, record.objectID)
		}
		grouped[record.objectID] = append(grouped[record.objectID], record)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	targetSet := make(map[string]int, len(targetEndpoints))
	for idx, endpoint := range targetEndpoints {
		targetSet[endpoint] = idx
	}

	var changed int64
	for _, objectID := range order {
		placements := grouped[objectID]
		if len(placements) == 0 {
			continue
		}

		sourceEndpoint := ""
		locator := placements[0].locator
		existingByEndpoint := make(map[string]placementRecord, len(placements))
		for _, placement := range placements {
			if placement.endpoint != "" {
				existingByEndpoint[placement.endpoint] = placement
			}
			if placement.endpoint == "" || sourceEndpoint != "" {
				continue
			}
			exists, err := s.replicaExists(ctx, placement.endpoint, placement.locator)
			if err == nil && exists {
				sourceEndpoint = placement.endpoint
			}
		}

		for _, placement := range placements {
			if placement.endpoint == "" {
				continue
			}
			if _, ok := targetSet[placement.endpoint]; ok {
				continue
			}
			_ = s.store.deleteOne(ctx, placement.endpoint, placement.locator)
			if _, err := s.db.Exec(ctx, `DELETE FROM object_placements WHERE id = $1::uuid`, placement.id); err != nil {
				return changed, err
			}
			changed++
		}

		if sourceEndpoint == "" {
			continue
		}

		for _, endpoint := range targetEndpoints {
			existing, ok := existingByEndpoint[endpoint]
			if ok {
				if existing.nodeID == "" {
					if _, err := s.db.Exec(ctx, `
						UPDATE object_placements
						SET storage_node_id = (SELECT id FROM storage_nodes WHERE endpoint = $2 LIMIT 1),
						    metadata = jsonb_build_object('endpoint', $2),
						    state = 'stored',
						    updated_at = NOW()
						WHERE id = $1::uuid
					`, existing.id, endpoint); err != nil {
						return changed, err
					}
					changed++
				}
				continue
			}

			if err := s.store.CopyBetweenEndpoints(ctx, sourceEndpoint, endpoint, locator); err != nil {
				continue
			}
			if _, err := s.db.Exec(ctx, `
				INSERT INTO object_placements (object_id, replica_index, chunk_ordinal, storage_node_id, locator, state, metadata, checksum_sha256, updated_at)
				VALUES (
					$1::uuid,
					$2,
					0,
					(SELECT id FROM storage_nodes WHERE endpoint = $3 LIMIT 1),
					$4,
					'stored',
					jsonb_build_object('endpoint', $3),
					'',
					NOW()
				)
			`, objectID, targetSet[endpoint], endpoint, locator); err != nil {
				return changed, err
			}
			changed++
		}
	}

	return changed, nil
}

func (s *TopologyService) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, endpoint, backend_type, zone, status, operator_state, capacity_bytes, used_bytes, metadata, COALESCE(last_heartbeat_at::text, ''), created_at::text, updated_at::text
		FROM storage_nodes
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Node, 0)
	for rows.Next() {
		var item Node
		var rawMetadata []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Endpoint, &item.BackendType, &item.Zone, &item.Status, &item.OperatorState, &item.CapacityBytes, &item.UsedBytes, &rawMetadata, &item.LastHeartbeatAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawMetadata, &item.Metadata); err != nil || len(item.Metadata) == 0 {
			item.Metadata = nil
		}
		items = append(items, item)
	}
	if items == nil {
		return []Node{}, nil
	}
	return items, rows.Err()
}

func (s *TopologyService) UpdateNodeOperatorState(ctx context.Context, nodeID, operatorState string) (Node, error) {
	operatorState = strings.ToLower(strings.TrimSpace(operatorState))
	if operatorState != "active" && operatorState != "draining" && operatorState != "maintenance" {
		return Node{}, ErrInvalidOperatorState
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE storage_nodes
		SET operator_state = $2, updated_at = NOW()
		WHERE id = $1::uuid
	`, nodeID, operatorState)
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() == 0 {
		return Node{}, pgx.ErrNoRows
	}

	nodes, err := s.ListNodes(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, node := range nodes {
		if node.ID == nodeID {
			return node, nil
		}
	}
	return Node{}, pgx.ErrNoRows
}

func (s *TopologyService) RePinNodeTLSIdentity(ctx context.Context, nodeID string) (Node, error) {
	node, err := s.nodeByID(ctx, nodeID)
	if err != nil {
		return Node{}, err
	}

	probe, err := s.fetchNodeHealth(ctx, node.Endpoint)
	if err != nil {
		return Node{}, err
	}
	if strings.TrimSpace(probe.TLSFingerprintSHA256) == "" {
		return Node{}, ErrNodeTLSIdentityUnavailable
	}

	metadataMap := map[string]any{}
	for key, value := range node.Metadata {
		metadataMap[key] = value
	}
	metadataMap["reachable"] = true
	metadataMap["service"] = probe.Health.Service
	metadataMap["fileCount"] = probe.Health.FileCount
	metadataMap["tlsExpectedFingerprintSha256"] = probe.TLSFingerprintSHA256
	metadataMap["tlsObservedFingerprintSha256"] = probe.TLSFingerprintSHA256
	metadataMap["tlsIdentityStatus"] = "verified"
	if probe.TLSCommonName != "" {
		metadataMap["tlsCommonName"] = probe.TLSCommonName
	}

	rawMetadata, err := json.Marshal(metadataMap)
	if err != nil {
		return Node{}, err
	}

	heartbeat := time.Now().UTC()
	tag, err := s.db.Exec(ctx, `
		UPDATE storage_nodes
		SET status = 'healthy',
		    capacity_bytes = $2,
		    used_bytes = $3,
		    metadata = $4::jsonb,
		    last_heartbeat_at = $5,
		    updated_at = NOW()
		WHERE id = $1::uuid
	`, nodeID, probe.Health.CapacityBytes, probe.Health.UsedBytes, string(rawMetadata), heartbeat)
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() == 0 {
		return Node{}, pgx.ErrNoRows
	}
	return s.nodeByID(ctx, nodeID)
}

func (s *TopologyService) ListPlacements(ctx context.Context, filter PlacementFilter) ([]Placement, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT
			p.id::text,
			p.object_id::text,
			o.bucket_id::text,
			o.object_key,
			COALESCE(o.version_id::text, ''),
			p.replica_index,
			p.chunk_ordinal,
			COALESCE(p.storage_node_id::text, ''),
			COALESCE(n.name, ''),
			p.locator,
			p.checksum_sha256,
			p.state,
			p.metadata,
			p.created_at::text,
			p.updated_at::text
		FROM object_placements p
		JOIN objects o ON o.id = p.object_id
		LEFT JOIN storage_nodes n ON n.id = p.storage_node_id
		WHERE ($1 = '' OR o.bucket_id::text = $1)
		  AND ($2 = '' OR o.object_key = $2)
		  AND ($3 = '' OR o.version_id::text = $3)
		ORDER BY o.created_at DESC, p.replica_index ASC, p.chunk_ordinal ASC
		LIMIT $4
	`, strings.TrimSpace(filter.BucketID), strings.TrimSpace(filter.Key), strings.TrimSpace(filter.VersionID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Placement, 0)
	for rows.Next() {
		var item Placement
		var rawMetadata []byte
		if err := rows.Scan(&item.ID, &item.ObjectID, &item.BucketID, &item.ObjectKey, &item.VersionID, &item.ReplicaIndex, &item.ChunkOrdinal, &item.StorageNodeID, &item.NodeName, &item.Locator, &item.ChecksumSHA256, &item.State, &rawMetadata, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawMetadata, &item.Metadata); err != nil || len(item.Metadata) == 0 {
			item.Metadata = nil
		}
		items = append(items, item)
	}
	if items == nil {
		return []Placement{}, nil
	}
	return items, rows.Err()
}

func nodeNameForEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	if parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return fmt.Sprintf("node-%s", strings.ReplaceAll(strings.TrimSpace(raw), ":", "-"))
}

func (s *TopologyService) nodeByID(ctx context.Context, nodeID string) (Node, error) {
	var item Node
	var rawMetadata []byte
	if err := s.db.QueryRow(ctx, `
		SELECT id::text, name, endpoint, backend_type, zone, status, operator_state, capacity_bytes, used_bytes, metadata, COALESCE(last_heartbeat_at::text, ''), created_at::text, updated_at::text
		FROM storage_nodes
		WHERE id = $1::uuid
	`, nodeID).Scan(&item.ID, &item.Name, &item.Endpoint, &item.BackendType, &item.Zone, &item.Status, &item.OperatorState, &item.CapacityBytes, &item.UsedBytes, &rawMetadata, &item.LastHeartbeatAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Node{}, err
	}
	if err := json.Unmarshal(rawMetadata, &item.Metadata); err != nil || len(item.Metadata) == 0 {
		item.Metadata = nil
	}
	return item, nil
}

func hashJoinToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func randomJoinToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *TopologyService) fetchNodeHealth(ctx context.Context, endpoint string) (nodeProbeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/healthz", nil)
	if err != nil {
		return nodeProbeResult{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nodeProbeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nodeProbeResult{}, fmt.Errorf("unexpected node health status %s", resp.Status)
	}
	var payload nodeHealthPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nodeProbeResult{}, err
	}
	result := nodeProbeResult{Health: payload}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		sum := sha256.Sum256(cert.Raw)
		result.TLSFingerprintSHA256 = hex.EncodeToString(sum[:])
		result.TLSCommonName = cert.Subject.CommonName
	}
	return result, nil
}

func (s *TopologyService) replicaExists(ctx context.Context, endpoint, locator string) (bool, error) {
	if s.store == nil {
		return false, errors.New("distributed store unavailable")
	}
	exists, err := s.store.ExistsOnEndpoint(ctx, endpoint, locator)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return exists, nil
}

func (s *TopologyService) syncStoreTokens(ctx context.Context) error {
	if s.store == nil || s.sealer == nil {
		return nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT endpoint, rpc_token_ciphertext
		FROM storage_nodes
		WHERE rpc_token_ciphertext <> ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var endpoint string
		var ciphertext string
		if err := rows.Scan(&endpoint, &ciphertext); err != nil {
			return err
		}
		token, err := s.sealer.OpenString(ciphertext)
		if err != nil {
			return err
		}
		s.store.SetEndpointToken(endpoint, token)
	}
	return rows.Err()
}

func (s *TopologyService) generateRPCCredential() (plaintext, tokenHash, ciphertext string, err error) {
	plaintext, err = randomJoinToken()
	if err != nil {
		return "", "", "", err
	}
	if s.sealer == nil {
		return "", "", "", errors.New("storage node sealer unavailable")
	}
	ciphertext, err = s.sealer.SealString(plaintext)
	if err != nil {
		return "", "", "", err
	}
	return plaintext, auth.HashSecret(plaintext), ciphertext, nil
}

func (s *TopologyService) targetReplicaCount(ctx context.Context) (int64, error) {
	var activeNodes int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM storage_nodes WHERE operator_state = 'active'`).Scan(&activeNodes); err != nil {
		return 0, err
	}
	if activeNodes == 0 {
		return 0, nil
	}
	return int64(s.effectiveReplicaCount(int(activeNodes))), nil
}

func (s *TopologyService) effectiveReplicaCount(available int) int {
	if available <= 0 {
		return 0
	}
	if s.replicaCount <= 0 || s.replicaCount > available {
		return available
	}
	return s.replicaCount
}

func (s *TopologyService) activeTargetEndpoints(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT endpoint
		FROM storage_nodes
		WHERE operator_state = 'active'
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	endpoints := make([]string, 0)
	for rows.Next() {
		var endpoint string
		if err := rows.Scan(&endpoint); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	replicaTarget := s.effectiveReplicaCount(len(endpoints))
	if replicaTarget == 0 {
		return []string{}, nil
	}
	return append([]string(nil), endpoints[:replicaTarget]...), nil
}
