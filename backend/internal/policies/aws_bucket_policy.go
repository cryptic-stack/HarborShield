package policies

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

var (
	ErrBucketPolicyNotFound = errors.New("bucket policy not found")
	ErrInvalidBucketPolicy  = errors.New("bucket policy must be valid AWS-style JSON")
)

type PolicyDecision string

const (
	PolicyDecisionNone  PolicyDecision = "none"
	PolicyDecisionAllow PolicyDecision = "allow"
	PolicyDecisionDeny  PolicyDecision = "deny"
)

type bucketPolicyDocument struct {
	Version   string                  `json:"Version"`
	ID        string                  `json:"Id,omitempty"`
	Statement []bucketPolicyStatement `json:"Statement"`
}

type bucketPolicyStatement struct {
	Sid          string          `json:"Sid,omitempty"`
	Effect       string          `json:"Effect"`
	Principal    json.RawMessage `json:"Principal,omitempty"`
	NotPrincipal json.RawMessage `json:"NotPrincipal,omitempty"`
	Action       json.RawMessage `json:"Action"`
	Resource     json.RawMessage `json:"Resource"`
	Condition    json.RawMessage `json:"Condition,omitempty"`
}

type normalizedBucketPolicyDocument struct {
	Version   string
	ID        string
	Statement []normalizedBucketPolicyStatement
}

type normalizedBucketPolicyStatement struct {
	Sid           string
	Effect        string
	Principals    []string
	NotPrincipals []string
	Actions       []string
	Resources     []string
	Conditions    []bucketPolicyCondition
}

type bucketPolicyCondition struct {
	Operator string
	Key      string
	Values   []string
}

func (s *Service) PutBucketPolicy(ctx context.Context, bucketID string, document []byte) ([]byte, error) {
	normalized, canonical, err := normalizeBucketPolicy(document)
	if err != nil {
		return nil, err
	}
	if len(normalized.Statement) == 0 {
		return nil, ErrInvalidBucketPolicy
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO bucket_policies (bucket_id, document, updated_at)
		VALUES ($1::uuid, $2::jsonb, NOW())
		ON CONFLICT (bucket_id)
		DO UPDATE SET document = EXCLUDED.document, updated_at = NOW()
	`, bucketID, canonical)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func (s *Service) GetBucketPolicy(ctx context.Context, bucketID string) ([]byte, error) {
	var raw []byte
	if err := s.db.QueryRow(ctx, `
		SELECT document::text
		FROM bucket_policies
		WHERE bucket_id = $1::uuid
	`, bucketID).Scan(&raw); err != nil {
		return nil, ErrBucketPolicyNotFound
	}
	return raw, nil
}

func (s *Service) DeleteBucketPolicy(ctx context.Context, bucketID string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM bucket_policies
		WHERE bucket_id = $1::uuid
	`, bucketID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBucketPolicyNotFound
	}
	return nil
}

func (s *Service) EvaluateBucketPolicy(ctx context.Context, bucketID, principal, action, resource string, requestContext map[string]string) (PolicyDecision, error) {
	raw, err := s.GetBucketPolicy(ctx, bucketID)
	if err != nil {
		if errors.Is(err, ErrBucketPolicyNotFound) {
			return PolicyDecisionNone, nil
		}
		return PolicyDecisionNone, err
	}
	document, _, err := normalizeBucketPolicy(raw)
	if err != nil {
		return PolicyDecisionNone, err
	}
	return evaluateBucketPolicy(document, principal, action, resource, requestContext), nil
}

func normalizeBucketPolicy(document []byte) (normalizedBucketPolicyDocument, []byte, error) {
	var parsed bucketPolicyDocument
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return normalizedBucketPolicyDocument{}, nil, ErrInvalidBucketPolicy
	}
	if strings.TrimSpace(parsed.Version) == "" {
		parsed.Version = "2012-10-17"
	}
	if parsed.Version != "2012-10-17" {
		return normalizedBucketPolicyDocument{}, nil, ErrInvalidBucketPolicy
	}
	if len(parsed.Statement) == 0 {
		return normalizedBucketPolicyDocument{}, nil, ErrInvalidBucketPolicy
	}

	normalized := normalizedBucketPolicyDocument{
		Version: parsed.Version,
		ID:      parsed.ID,
	}
	for _, statement := range parsed.Statement {
		next, err := normalizeBucketPolicyStatement(statement)
		if err != nil {
			return normalizedBucketPolicyDocument{}, nil, err
		}
		normalized.Statement = append(normalized.Statement, next)
	}

	canonical, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return normalizedBucketPolicyDocument{}, nil, fmt.Errorf("marshal bucket policy: %w", err)
	}
	return normalized, canonical, nil
}

func normalizeBucketPolicyStatement(statement bucketPolicyStatement) (normalizedBucketPolicyStatement, error) {
	effect := strings.Title(strings.ToLower(strings.TrimSpace(statement.Effect)))
	if effect != "Allow" && effect != "Deny" {
		return normalizedBucketPolicyStatement{}, ErrInvalidBucketPolicy
	}
	principals, err := normalizeBucketPolicyPrincipal(statement.Principal)
	if err != nil {
		return normalizedBucketPolicyStatement{}, err
	}
	notPrincipals, err := normalizeBucketPolicyPrincipal(statement.NotPrincipal)
	if err != nil {
		return normalizedBucketPolicyStatement{}, err
	}
	if len(principals) == 0 && len(notPrincipals) == 0 {
		return normalizedBucketPolicyStatement{}, ErrInvalidBucketPolicy
	}
	if len(principals) > 0 && len(notPrincipals) > 0 {
		return normalizedBucketPolicyStatement{}, ErrInvalidBucketPolicy
	}
	actions, err := normalizeStringOrList(statement.Action)
	if err != nil || len(actions) == 0 {
		return normalizedBucketPolicyStatement{}, ErrInvalidBucketPolicy
	}
	resources, err := normalizeStringOrList(statement.Resource)
	if err != nil || len(resources) == 0 {
		return normalizedBucketPolicyStatement{}, ErrInvalidBucketPolicy
	}
	conditions, err := normalizeBucketPolicyConditions(statement.Condition)
	if err != nil {
		return normalizedBucketPolicyStatement{}, err
	}
	return normalizedBucketPolicyStatement{
		Sid:           statement.Sid,
		Effect:        effect,
		Principals:    principals,
		NotPrincipals: notPrincipals,
		Actions:       actions,
		Resources:     resources,
		Conditions:    conditions,
	}, nil
}

func normalizeBucketPolicyConditions(raw json.RawMessage) ([]bucketPolicyCondition, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var operators map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &operators); err != nil {
		return nil, ErrInvalidBucketPolicy
	}
	conditions := make([]bucketPolicyCondition, 0)
	for operator, entries := range operators {
		switch operator {
		case "IpAddress", "NotIpAddress", "StringEquals", "StringLike", "StringNotEquals", "StringNotLike":
		default:
			return nil, ErrInvalidBucketPolicy
		}
		for key, rawValues := range entries {
			values, err := normalizeStringOrList(rawValues)
			if err != nil || len(values) == 0 {
				return nil, ErrInvalidBucketPolicy
			}
			conditions = append(conditions, bucketPolicyCondition{
				Operator: operator,
				Key:      key,
				Values:   values,
			})
		}
	}
	return conditions, nil
}

func normalizeBucketPolicyPrincipal(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var wildcard string
	if err := json.Unmarshal(raw, &wildcard); err == nil {
		if wildcard == "*" {
			return []string{"*"}, nil
		}
		return nil, ErrInvalidBucketPolicy
	}
	var principal struct {
		AWS json.RawMessage `json:"AWS"`
	}
	if err := json.Unmarshal(raw, &principal); err != nil {
		return nil, ErrInvalidBucketPolicy
	}
	values, err := normalizeStringOrList(principal.AWS)
	if err != nil || len(values) == 0 {
		return nil, ErrInvalidBucketPolicy
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizePolicyPrincipal(value)
		if value == "" {
			return nil, ErrInvalidBucketPolicy
		}
		out = append(out, value)
	}
	return out, nil
}

func normalizeStringOrList(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, ErrInvalidBucketPolicy
		}
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, ErrInvalidBucketPolicy
	}
	out := make([]string, 0, len(many))
	for _, item := range many {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, ErrInvalidBucketPolicy
		}
		out = append(out, item)
	}
	return out, nil
}

func normalizePolicyPrincipal(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "*":
		return "*"
	case strings.HasPrefix(value, "arn:aws:iam::"):
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	default:
		return value
	}
}

func evaluateBucketPolicy(document normalizedBucketPolicyDocument, principal, action, resource string, requestContext map[string]string) PolicyDecision {
	if principal == "" {
		principal = "*"
	}
	allowed := false
	for _, statement := range document.Statement {
		if !bucketPolicyStatementPrincipalMatches(statement, principal) {
			continue
		}
		if !bucketPolicyAnyMatches(statement.Actions, action) {
			continue
		}
		if !bucketPolicyAnyMatches(statement.Resources, resource) {
			continue
		}
		if !bucketPolicyConditionsMatch(statement.Conditions, requestContext) {
			continue
		}
		if statement.Effect == "Deny" {
			return PolicyDecisionDeny
		}
		if statement.Effect == "Allow" {
			allowed = true
		}
	}
	if allowed {
		return PolicyDecisionAllow
	}
	return PolicyDecisionNone
}

func bucketPolicyStatementPrincipalMatches(statement normalizedBucketPolicyStatement, principal string) bool {
	if len(statement.Principals) > 0 {
		return bucketPolicyPrincipalMatches(statement.Principals, principal)
	}
	if len(statement.NotPrincipals) > 0 {
		return !bucketPolicyPrincipalMatches(statement.NotPrincipals, principal)
	}
	return false
}

func bucketPolicyPrincipalMatches(principals []string, principal string) bool {
	for _, candidate := range principals {
		if candidate == "*" || candidate == principal {
			return true
		}
	}
	return false
}

func bucketPolicyAnyMatches(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if match(pattern, value) {
			return true
		}
	}
	return false
}

func bucketPolicyConditionsMatch(conditions []bucketPolicyCondition, requestContext map[string]string) bool {
	for _, condition := range conditions {
		actual := strings.TrimSpace(requestContext[condition.Key])
		switch condition.Operator {
		case "StringEquals":
			if !bucketPolicyAnyMatches(condition.Values, actual) {
				return false
			}
		case "StringLike":
			if !bucketPolicyAnyMatches(condition.Values, actual) {
				return false
			}
		case "StringNotEquals":
			if bucketPolicyAnyMatches(condition.Values, actual) {
				return false
			}
		case "StringNotLike":
			if bucketPolicyAnyMatches(condition.Values, actual) {
				return false
			}
		case "IpAddress":
			if !bucketPolicyAnyIPMatches(condition.Values, actual) {
				return false
			}
		case "NotIpAddress":
			if bucketPolicyAnyIPMatches(condition.Values, actual) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func bucketPolicyAnyIPMatches(patterns []string, value string) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	for _, pattern := range patterns {
		if parsed := net.ParseIP(pattern); parsed != nil && parsed.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(pattern); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
