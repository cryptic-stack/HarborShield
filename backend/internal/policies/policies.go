package policies

import "strings"

type Statement struct {
	Subject    string
	Action     string
	Resource   string
	Effect     string
	Conditions map[string]string
}

func Evaluate(statements []Statement, subject, action, resource string) bool {
	allowed := false
	for _, stmt := range statements {
		if stmt.Subject != subject && stmt.Subject != "*" {
			continue
		}
		if !match(stmt.Action, action) || !match(stmt.Resource, resource) {
			continue
		}
		if strings.EqualFold(stmt.Effect, "deny") {
			return false
		}
		if strings.EqualFold(stmt.Effect, "allow") {
			allowed = true
		}
	}
	return allowed
}

func match(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}
