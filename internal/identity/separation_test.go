package identity

import (
	"strings"
	"testing"
	"time"
)

func TestPolicyAuthorCannotApprove(t *testing.T) {
	rule := PolicyAuthorCannotApprove()

	// An identity with policy_author role should be blocked from approving.
	author := Identity{
		ID:   "user-author",
		Type: "human",
		Name: "Author Alice",
		Roles: []Role{
			{Name: "policy_author", Scope: "team", ScopeID: "team-1"},
		},
	}

	err := rule.Check(author, "approve_policy")
	if err == nil {
		t.Fatal("expected error for policy author approving")
	}
	if !strings.Contains(err.Error(), "policy_author") {
		t.Fatalf("unexpected error: %v", err)
	}

	// A non-author identity should be allowed.
	reviewer := Identity{
		ID:   "user-reviewer",
		Type: "human",
		Name: "Reviewer Bob",
		Roles: []Role{
			{Name: "approver", Scope: "team", ScopeID: "team-1"},
		},
	}

	if err := rule.Check(reviewer, "approve_policy"); err != nil {
		t.Fatalf("expected no error for approver: %v", err)
	}

	// Any action other than approve_policy should pass.
	if err := rule.Check(author, "read_policy"); err != nil {
		t.Fatalf("expected no error for non-approve action: %v", err)
	}
}

func TestBreakGlassRequiresPostmortem(t *testing.T) {
	rule := BreakGlassRequiresPostmortem()

	actor := Identity{
		ID:   "ops-1",
		Type: "human",
		Name: "On-call Engineer",
		Roles: []Role{
			{Name: "operator", Scope: "org", ScopeID: "org-1"},
		},
	}

	// break_glass action should always produce an error requiring review.
	err := rule.Check(actor, "break_glass")
	if err == nil {
		t.Fatal("expected error for break-glass action")
	}
	if !strings.Contains(err.Error(), "post-incident review") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-break-glass actions should pass.
	if err := rule.Check(actor, "deploy"); err != nil {
		t.Fatalf("expected no error for non-break-glass action: %v", err)
	}
}

func TestExceptionGrantsTimeBound(t *testing.T) {
	rule := ExceptionGrantsTimeBound()

	actor := Identity{
		ID:   "admin-1",
		Type: "human",
		Name: "Admin Charlie",
		Roles: []Role{
			{Name: "admin", Scope: "org", ScopeID: "org-1"},
		},
	}

	// grant_exception should always trigger a reminder about expiry.
	err := rule.Check(actor, "grant_exception")
	if err == nil {
		t.Fatal("expected error for grant_exception action")
	}
	if !strings.Contains(err.Error(), "expiry") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-exception actions should pass.
	if err := rule.Check(actor, "read"); err != nil {
		t.Fatalf("expected no error for non-exception action: %v", err)
	}

	// ValidateException with zero expiry
	exc := TemporaryException{
		GrantedTo: "user-1",
		Privilege: "admin",
		Reason:    "incident",
	}
	if err := ValidateException(exc); err == nil {
		t.Fatal("expected error for zero expiry")
	}

	// ValidateException with past expiry
	exc.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if err := ValidateException(exc); err == nil {
		t.Fatal("expected error for past expiry")
	}

	// ValidateException with future expiry should pass
	exc.ExpiresAt = time.Now().Add(1 * time.Hour)
	if err := ValidateException(exc); err != nil {
		t.Fatalf("expected no error for future expiry: %v", err)
	}
}

func TestConnectorAdminNotSessionOperator(t *testing.T) {
	rule := ConnectorAdminNotSessionOperator()

	// An admin trying to operate a session should be blocked.
	admin := Identity{
		ID:   "admin-user",
		Type: "human",
		Name: "Admin",
		Roles: []Role{
			{Name: "admin", Scope: "connector", ScopeID: "conn-1"},
		},
	}

	err := rule.Check(admin, "operate_session")
	if err == nil {
		t.Fatal("expected error for admin operating session")
	}
	if !strings.Contains(err.Error(), "admin role") {
		t.Fatalf("expected error about admin role, got: %v", err)
	}
	if !strings.Contains(err.Error(), admin.ID) {
		t.Fatalf("expected error to include actor ID %q, got: %v", admin.ID, err)
	}

	// An admin performing a non-session action should be allowed.
	if err := rule.Check(admin, "configure_connector"); err != nil {
		t.Fatalf("expected no error for non-session action, got: %v", err)
	}

	// A non-admin operating a session should be allowed.
	operator := Identity{
		ID:   "op-user",
		Type: "human",
		Name: "Operator",
		Roles: []Role{
			{Name: "operator", Scope: "connector", ScopeID: "conn-1"},
		},
	}
	if err := rule.Check(operator, "operate_session"); err != nil {
		t.Fatalf("expected no error for non-admin operator, got: %v", err)
	}
}

func TestEvaluateRules(t *testing.T) {
	rules := DefaultRules()

	// A clean actor performing a normal action should pass all rules.
	clean := Identity{
		ID:   "user-clean",
		Type: "human",
		Name: "Clean User",
		Roles: []Role{
			{Name: "viewer", Scope: "org", ScopeID: "org-1"},
		},
	}

	if err := EvaluateRules(rules, clean, "read"); err != nil {
		t.Fatalf("expected no error for clean actor: %v", err)
	}

	// A policy author trying to approve should be caught.
	author := Identity{
		ID:   "user-author",
		Type: "human",
		Name: "Author",
		Roles: []Role{
			{Name: "policy_author", Scope: "team", ScopeID: "team-1"},
		},
	}

	if err := EvaluateRules(rules, author, "approve_policy"); err == nil {
		t.Fatal("expected error from EvaluateRules for policy author approving")
	}
}
