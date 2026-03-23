package authz

import (
	"context"
	"fmt"
)

type PolicyAuthorizer interface {
	AuthorizeRole(ctx context.Context, role, action, resource string) (bool, error)
	AuthorizeSubject(ctx context.Context, subjectType, subjectID, fallbackRole, action, resource string) (bool, string, error)
}

type Authorizer struct {
	policies PolicyAuthorizer
}

func New(policyService PolicyAuthorizer) *Authorizer {
	return &Authorizer{policies: policyService}
}

func (a *Authorizer) CheckRole(ctx context.Context, role, action, resource string) error {
	allowed, err := a.policies.AuthorizeRole(ctx, role, action, resource)
	if err != nil {
		return fmt.Errorf("authorize role %s: %w", role, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (a *Authorizer) CheckSubject(ctx context.Context, subjectType, subjectID, fallbackRole, action, resource string) (string, error) {
	allowed, resolvedRole, err := a.policies.AuthorizeSubject(ctx, subjectType, subjectID, fallbackRole, action, resource)
	if err != nil {
		return "", fmt.Errorf("authorize subject %s/%s: %w", subjectType, subjectID, err)
	}
	if !allowed {
		return resolvedRole, ErrForbidden
	}
	return resolvedRole, nil
}
