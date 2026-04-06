package resource

import "testing"

func TestPolicyEngineDefaultDecision(t *testing.T) {
	engine := NewResourcePolicyEngine(nil, "block")
	res := Resource{Type: ResourceRepo, Provider: "github", Environment: "dev"}
	if got := engine.Evaluate(res, VerbRead); got != DecisionBlock {
		t.Errorf("expected block default, got %s", got)
	}
}

func TestPolicyEngineInvalidDefault(t *testing.T) {
	engine := NewResourcePolicyEngine(nil, "invalid")
	res := Resource{Type: ResourceRepo}
	if got := engine.Evaluate(res, VerbRead); got != DecisionBlock {
		t.Errorf("invalid default should fall back to block, got %s", got)
	}
}

func TestPolicyEnginePriorityOrder(t *testing.T) {
	rules := []ResourceRule{
		{ResourceType: "repo", Decision: "block", Priority: 10},
		{ResourceType: "repo", Decision: "allow", Priority: 1},
	}
	engine := NewResourcePolicyEngine(rules, "block")
	res := Resource{Type: ResourceRepo}

	// Both match, but priority 1 (allow) is higher priority.
	// However, deny-overrides: block wins regardless.
	if got := engine.Evaluate(res, VerbRead); got != DecisionBlock {
		t.Errorf("expected block due to deny-overrides, got %s", got)
	}
}

func TestPolicyEngineDenyOverrides(t *testing.T) {
	rules := []ResourceRule{
		{Provider: "github", Decision: "allow", Priority: 1},
		{Provider: "github", Verb: "delete", Decision: "block", Priority: 100},
	}
	engine := NewResourcePolicyEngine(rules, "allow")
	res := Resource{Type: ResourceRepo, Provider: "github"}

	// Delete: both match, but block rule overrides.
	if got := engine.Evaluate(res, VerbDelete); got != DecisionBlock {
		t.Errorf("expected block for delete due to deny-overrides, got %s", got)
	}

	// Read: only the allow rule matches.
	if got := engine.Evaluate(res, VerbRead); got != DecisionAllow {
		t.Errorf("expected allow for read, got %s", got)
	}
}

func TestPolicyEngineNoMatchUsesDefault(t *testing.T) {
	rules := []ResourceRule{
		{Provider: "github", Decision: "allow", Priority: 1},
	}
	engine := NewResourcePolicyEngine(rules, "review")
	res := Resource{Type: ResourceTable, Provider: "postgres"}

	if got := engine.Evaluate(res, VerbRead); got != DecisionReview {
		t.Errorf("expected review default, got %s", got)
	}
}

func TestPolicyEngineEnvironmentInheritance(t *testing.T) {
	rules := []ResourceRule{
		{Environment: "prod", Verb: "delete", Decision: "block", Priority: 1},
	}
	engine := NewResourcePolicyEngine(rules, "allow")

	prodRes := Resource{Environment: "prod"}
	if got := engine.Evaluate(prodRes, VerbDelete); got != DecisionBlock {
		t.Errorf("expected block for prod delete, got %s", got)
	}

	// Staging should inherit the prod rule.
	stagingRes := Resource{Environment: "staging"}
	if got := engine.Evaluate(stagingRes, VerbDelete); got != DecisionBlock {
		t.Errorf("expected block for staging delete (inherited from prod), got %s", got)
	}

	// Dev should not be affected.
	devRes := Resource{Environment: "dev"}
	if got := engine.Evaluate(devRes, VerbDelete); got != DecisionAllow {
		t.Errorf("expected allow for dev delete (no inheritance), got %s", got)
	}
}

func TestPolicyEngineExplicitStagingOverridesInheritance(t *testing.T) {
	rules := []ResourceRule{
		{Environment: "prod", Verb: "delete", Decision: "block", Priority: 1},
		{Environment: "staging", Verb: "delete", Decision: "review", Priority: 2},
	}
	engine := NewResourcePolicyEngine(rules, "allow")

	stagingRes := Resource{Environment: "staging"}
	// Explicit staging rule should be used instead of inherited prod rule.
	// But deny-overrides means block still wins... unless the prod rule
	// does NOT generate an inherited staging rule because one already exists.
	// Since we have explicit staging, no inheritance happens.
	if got := engine.Evaluate(stagingRes, VerbDelete); got != DecisionReview {
		t.Errorf("expected review for staging (explicit rule), got %s", got)
	}
}

func TestPolicyEngineMultipleRulesReview(t *testing.T) {
	rules := []ResourceRule{
		{ResourceType: "table", Sensitivity: "confidential", Decision: "review", Priority: 1},
		{ResourceType: "table", Decision: "allow", Priority: 10},
	}
	engine := NewResourcePolicyEngine(rules, "block")

	confTable := Resource{Type: ResourceTable, Sensitivity: SensitivityConfidential}
	if got := engine.Evaluate(confTable, VerbRead); got != DecisionReview {
		t.Errorf("expected review for confidential table, got %s", got)
	}

	pubTable := Resource{Type: ResourceTable, Sensitivity: SensitivityPublic}
	if got := engine.Evaluate(pubTable, VerbRead); got != DecisionAllow {
		t.Errorf("expected allow for public table, got %s", got)
	}
}
