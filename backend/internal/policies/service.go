package policies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

var (
	ErrInvalidRoleName        = errors.New("role does not exist")
	ErrInvalidEffect          = errors.New("effect must be allow or deny")
	ErrInvalidAction          = errors.New("action is required and must not contain spaces")
	ErrInvalidResource        = errors.New("resource is required and must not contain spaces")
	ErrInvalidSubjectID       = errors.New("subjectId is required")
	ErrInvalidSubjectType     = errors.New("subjectType must be one of user, credential, or admin_token")
	ErrProtectedRoleStatement = errors.New("superadmin statements cannot be modified")
	ErrStatementNotFound      = errors.New("statement not found")
	ErrBindingNotFound        = errors.New("binding not found")
)

type Role struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Builtin     bool              `json:"builtin"`
	Statements  []StatementRecord `json:"statements"`
}

type StatementRecord struct {
	ID         string            `json:"id"`
	RoleName   string            `json:"roleName"`
	Action     string            `json:"action"`
	Resource   string            `json:"resource"`
	Effect     string            `json:"effect"`
	Conditions map[string]string `json:"conditions"`
}

type SubjectBinding struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Resource    string `json:"resource"`
	RoleName    string `json:"roleName"`
	CreatedAt   string `json:"createdAt"`
}

type EvaluationTrace struct {
	Role          string            `json:"role"`
	Allowed       bool              `json:"allowed"`
	ExplicitDeny  bool              `json:"explicitDeny"`
	MatchedScopes []SubjectBinding  `json:"matchedScopes,omitempty"`
	Statements    []StatementRecord `json:"statements"`
}

type EvaluationResult struct {
	SubjectType   string            `json:"subjectType"`
	SubjectID     string            `json:"subjectId"`
	Action        string            `json:"action"`
	Resource      string            `json:"resource"`
	FallbackRole  string            `json:"fallbackRole"`
	EffectiveRole string            `json:"effectiveRole"`
	Allowed       bool              `json:"allowed"`
	Reason        string            `json:"reason"`
	Bindings      []SubjectBinding  `json:"bindings"`
	Traces        []EvaluationTrace `json:"traces"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) AuthorizeRole(ctx context.Context, role, action, resource string) (bool, error) {
	statements, err := s.ListStatementsForRole(ctx, role)
	if err != nil {
		return false, err
	}
	if len(statements) == 0 {
		return false, nil
	}
	evalStatements := make([]Statement, 0, len(statements))
	for _, stmt := range statements {
		evalStatements = append(evalStatements, Statement{
			Subject:    role,
			Action:     stmt.Action,
			Resource:   stmt.Resource,
			Effect:     stmt.Effect,
			Conditions: stmt.Conditions,
		})
	}
	return Evaluate(evalStatements, role, action, resource), nil
}

func (s *Service) AuthorizeSubject(ctx context.Context, subjectType, subjectID, fallbackRole, action, resource string) (bool, string, error) {
	roleName, err := s.ResolveRole(ctx, subjectType, subjectID, fallbackRole, resource)
	if err != nil {
		return false, "", err
	}
	boundRoles, err := s.rolesForSubjectResource(ctx, subjectType, subjectID, resource)
	if err != nil {
		return false, "", err
	}
	for _, boundRole := range boundRoles {
		allowed, err := s.AuthorizeRole(ctx, boundRole, action, resource)
		if err != nil {
			return false, "", err
		}
		if allowed {
			return true, boundRole, nil
		}
	}
	allowed, err := s.AuthorizeRole(ctx, roleName, action, resource)
	return allowed, roleName, err
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.Query(ctx, `
		SELECT name, description, builtin
		FROM roles
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.Name, &role.Description, &role.Builtin); err != nil {
			return nil, err
		}
		statements, err := s.ListStatementsForRole(ctx, role.Name)
		if err != nil {
			return nil, err
		}
		role.Statements = statements
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *Service) ListStatementsForRole(ctx context.Context, roleName string) ([]StatementRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, role_name, action, resource, effect, conditions
		FROM role_policy_statements
		WHERE role_name = $1
		ORDER BY action, resource
	`, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []StatementRecord
	for rows.Next() {
		var record StatementRecord
		var rawConditions []byte
		if err := rows.Scan(&record.ID, &record.RoleName, &record.Action, &record.Resource, &record.Effect, &rawConditions); err != nil {
			return nil, err
		}
		record.Conditions, err = decodeConditions(rawConditions)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Service) CreateStatement(ctx context.Context, roleName, action, resource, effect string, conditions map[string]string) (StatementRecord, error) {
	if err := s.validateStatementInput(ctx, roleName, action, resource, effect); err != nil {
		return StatementRecord{}, err
	}
	rawConditions, err := json.Marshal(normalizeConditions(conditions))
	if err != nil {
		return StatementRecord{}, fmt.Errorf("marshal conditions: %w", err)
	}
	var record StatementRecord
	var storedConditions []byte
	err = s.db.QueryRow(ctx, `
		INSERT INTO role_policy_statements (role_name, action, resource, effect, conditions)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id::text, role_name, action, resource, effect, conditions
	`, roleName, action, resource, effect, rawConditions).
		Scan(&record.ID, &record.RoleName, &record.Action, &record.Resource, &record.Effect, &storedConditions)
	if err != nil {
		return StatementRecord{}, err
	}
	record.Conditions, err = decodeConditions(storedConditions)
	if err != nil {
		return StatementRecord{}, err
	}
	return record, nil
}

func (s *Service) UpdateStatement(ctx context.Context, statementID, action, resource, effect string, conditions map[string]string) (StatementRecord, error) {
	roleName, err := s.statementRole(ctx, statementID)
	if err != nil {
		return StatementRecord{}, err
	}
	if err := s.validateStatementInput(ctx, roleName, action, resource, effect); err != nil {
		return StatementRecord{}, err
	}
	rawConditions, err := json.Marshal(normalizeConditions(conditions))
	if err != nil {
		return StatementRecord{}, fmt.Errorf("marshal conditions: %w", err)
	}

	var record StatementRecord
	var storedConditions []byte
	err = s.db.QueryRow(ctx, `
		UPDATE role_policy_statements
		SET action = $2, resource = $3, effect = $4, conditions = $5::jsonb
		WHERE id = $1::uuid
		RETURNING id::text, role_name, action, resource, effect, conditions
	`, statementID, action, resource, effect, rawConditions).
		Scan(&record.ID, &record.RoleName, &record.Action, &record.Resource, &record.Effect, &storedConditions)
	if err != nil {
		return StatementRecord{}, err
	}
	record.Conditions, err = decodeConditions(storedConditions)
	if err != nil {
		return StatementRecord{}, err
	}
	return record, nil
}

func (s *Service) DeleteStatement(ctx context.Context, statementID string) error {
	roleName, err := s.statementRole(ctx, statementID)
	if err != nil {
		return err
	}
	if roleName == "superadmin" {
		return ErrProtectedRoleStatement
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM role_policy_statements
		WHERE id = $1::uuid
	`, statementID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStatementNotFound
	}
	return nil
}

func (s *Service) ResolveRole(ctx context.Context, subjectType, subjectID, fallbackRole, resource string) (string, error) {
	roles, err := s.rolesForSubjectResource(ctx, subjectType, subjectID, resource)
	if err != nil {
		return "", err
	}
	if len(roles) > 0 {
		return roles[0], nil
	}
	return fallbackRole, nil
}

func (s *Service) ListBindings(ctx context.Context) ([]SubjectBinding, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, subject_type, subject_id, resource, role_name, created_at::text
		FROM subject_role_bindings
		ORDER BY created_at DESC, resource ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SubjectBinding
	for rows.Next() {
		var item SubjectBinding
		if err := rows.Scan(&item.ID, &item.SubjectType, &item.SubjectID, &item.Resource, &item.RoleName, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) UpsertBinding(ctx context.Context, subjectType, subjectID, resource, roleName string) (SubjectBinding, error) {
	if err := s.validateBindingInput(ctx, subjectType, subjectID, resource, roleName); err != nil {
		return SubjectBinding{}, err
	}
	var item SubjectBinding
	err := s.db.QueryRow(ctx, `
		INSERT INTO subject_role_bindings (subject_type, subject_id, resource, role_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (subject_type, subject_id, resource)
		DO UPDATE SET role_name = EXCLUDED.role_name
		RETURNING id::text, subject_type, subject_id, resource, role_name, created_at::text
	`, subjectType, subjectID, resource, roleName).
		Scan(&item.ID, &item.SubjectType, &item.SubjectID, &item.Resource, &item.RoleName, &item.CreatedAt)
	return item, err
}

func (s *Service) DeleteBinding(ctx context.Context, bindingID string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM subject_role_bindings
		WHERE id = $1::uuid
	`, bindingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Service) ExplainSubjectAuthorization(ctx context.Context, subjectType, subjectID, fallbackRole, action, resource string) (EvaluationResult, error) {
	if err := validateActionResourceEffect(action, resource, "allow"); err != nil && !errors.Is(err, ErrInvalidEffect) {
		return EvaluationResult{}, err
	}
	if err := validateBindingFields(subjectType, subjectID, resource); err != nil {
		return EvaluationResult{}, err
	}

	result := EvaluationResult{
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		Action:       action,
		Resource:     resource,
		FallbackRole: fallbackRole,
	}

	allBindings, err := s.ListBindings(ctx)
	if err != nil {
		return EvaluationResult{}, err
	}
	for _, binding := range allBindings {
		if binding.SubjectType == subjectType && binding.SubjectID == subjectID && bindingMatchesResource(binding.Resource, resource) {
			result.Bindings = append(result.Bindings, binding)
		}
	}

	boundRoles, err := s.rolesForSubjectResource(ctx, subjectType, subjectID, resource)
	if err != nil {
		return EvaluationResult{}, err
	}
	bindingMap := make(map[string][]SubjectBinding)
	for _, binding := range result.Bindings {
		bindingMap[binding.RoleName] = append(bindingMap[binding.RoleName], binding)
	}

	for _, roleName := range boundRoles {
		trace, err := s.traceRole(ctx, roleName, action, resource, bindingMap[roleName])
		if err != nil {
			return EvaluationResult{}, err
		}
		result.Traces = append(result.Traces, trace)
		if trace.Allowed {
			result.Allowed = true
			result.EffectiveRole = roleName
			result.Reason = "allowed_by_binding"
			return result, nil
		}
	}

	fallbackTrace, err := s.traceRole(ctx, fallbackRole, action, resource, nil)
	if err != nil {
		return EvaluationResult{}, err
	}
	result.Traces = append(result.Traces, fallbackTrace)
	result.Allowed = fallbackTrace.Allowed
	result.EffectiveRole = fallbackRole
	if fallbackTrace.Allowed {
		result.Reason = "allowed_by_fallback_role"
	} else if fallbackTrace.ExplicitDeny {
		result.Reason = "denied_by_statement"
	} else {
		result.Reason = "denied_by_default"
	}
	return result, nil
}

func (s *Service) validateStatementInput(ctx context.Context, roleName, action, resource, effect string) error {
	if err := s.ensureRoleExists(ctx, roleName); err != nil {
		return err
	}
	if err := validateActionResourceEffect(action, resource, effect); err != nil {
		return err
	}
	if roleName == "superadmin" {
		return ErrProtectedRoleStatement
	}
	return nil
}

func (s *Service) validateBindingInput(ctx context.Context, subjectType, subjectID, resource, roleName string) error {
	if err := s.ensureRoleExists(ctx, roleName); err != nil {
		return err
	}
	return validateBindingFields(subjectType, subjectID, resource)
}

func (s *Service) ensureRoleExists(ctx context.Context, roleName string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM roles WHERE name = $1)
	`, roleName).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrInvalidRoleName
	}
	return nil
}

func (s *Service) statementRole(ctx context.Context, statementID string) (string, error) {
	var roleName string
	if err := s.db.QueryRow(ctx, `
		SELECT role_name
		FROM role_policy_statements
		WHERE id = $1::uuid
	`, statementID).Scan(&roleName); err != nil {
		return "", ErrStatementNotFound
	}
	return roleName, nil
}

func validateActionResourceEffect(action, resource, effect string) error {
	if strings.TrimSpace(action) == "" || strings.ContainsAny(action, " \t\r\n") {
		return ErrInvalidAction
	}
	if strings.TrimSpace(resource) == "" || strings.ContainsAny(resource, " \t\r\n") {
		return ErrInvalidResource
	}
	if !strings.EqualFold(effect, "allow") && !strings.EqualFold(effect, "deny") {
		return ErrInvalidEffect
	}
	return nil
}

func validateBindingFields(subjectType, subjectID, resource string) error {
	if strings.TrimSpace(subjectID) == "" {
		return ErrInvalidSubjectID
	}
	if strings.TrimSpace(resource) == "" || strings.ContainsAny(resource, " \t\r\n") {
		return ErrInvalidResource
	}
	switch subjectType {
	case "user", "credential", "admin_token":
		return nil
	default:
		return ErrInvalidSubjectType
	}
}

func (s *Service) rolesForSubjectResource(ctx context.Context, subjectType, subjectID, resource string) ([]string, error) {
	if subjectType == "" || subjectID == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT role_name, resource
		FROM subject_role_bindings
		WHERE subject_type = $1 AND subject_id = $2
		ORDER BY CASE WHEN resource = '*' THEN 1 ELSE 0 END, resource ASC
	`, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var roleName, resourcePattern string
		if err := rows.Scan(&roleName, &resourcePattern); err != nil {
			return nil, err
		}
		if bindingMatchesResource(resourcePattern, resource) {
			roles = append(roles, roleName)
		}
	}
	return roles, rows.Err()
}

func bindingMatchesResource(pattern, resource string) bool {
	return match(pattern, resource)
}

func (s *Service) traceRole(ctx context.Context, roleName, action, resource string, matchedScopes []SubjectBinding) (EvaluationTrace, error) {
	statements, err := s.ListStatementsForRole(ctx, roleName)
	if err != nil {
		return EvaluationTrace{}, err
	}
	allowed, explicitDeny, matchedStatements := evaluateRoleStatements(statements, action, resource)
	return EvaluationTrace{
		Role:          roleName,
		Allowed:       allowed,
		ExplicitDeny:  explicitDeny,
		MatchedScopes: matchedScopes,
		Statements:    matchedStatements,
	}, nil
}

func evaluateRoleStatements(statements []StatementRecord, action, resource string) (bool, bool, []StatementRecord) {
	matched := make([]StatementRecord, 0)
	allowed := false
	explicitDeny := false
	for _, stmt := range statements {
		if !match(stmt.Action, action) || !match(stmt.Resource, resource) {
			continue
		}
		matched = append(matched, stmt)
		if strings.EqualFold(stmt.Effect, "deny") {
			explicitDeny = true
		}
		if strings.EqualFold(stmt.Effect, "allow") {
			allowed = true
		}
	}
	if explicitDeny {
		return false, true, matched
	}
	return allowed, false, matched
}

func normalizeConditions(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func decodeConditions(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode conditions: %w", err)
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}
