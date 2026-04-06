package resource

import "testing"

func TestMatchExact(t *testing.T) {
	rule := ResourceRule{
		ResourceType: "repo",
		Provider:     "github",
		Environment:  "prod",
		Verb:         "delete",
		Decision:     "block",
	}
	res := Resource{
		Type:        ResourceRepo,
		Provider:    "github",
		Environment: "prod",
		Path:        []string{"myorg", "myrepo"},
		Sensitivity: SensitivityInternal,
	}

	if !Match(rule, res, VerbDelete) {
		t.Error("expected exact match to succeed")
	}
}

func TestMatchWildcards(t *testing.T) {
	rule := ResourceRule{
		ResourceType: "*",
		Provider:     "*",
		Environment:  "*",
		Verb:         "*",
		Decision:     "allow",
	}
	res := Resource{
		Type:        ResourceTable,
		Provider:    "postgres",
		Environment: "dev",
	}

	if !Match(rule, res, VerbRead) {
		t.Error("wildcard rule should match any resource")
	}
}

func TestMatchEmptyFieldsMatchAll(t *testing.T) {
	rule := ResourceRule{Decision: "allow"}
	res := Resource{
		Type:        ResourceFile,
		Provider:    "github",
		Environment: "staging",
	}

	if !Match(rule, res, VerbUpdate) {
		t.Error("empty fields in rule should match everything")
	}
}

func TestMatchGlobPath(t *testing.T) {
	rule := ResourceRule{
		PathPattern: "myorg/*/main",
		Decision:    "review",
	}
	res := Resource{
		Path: []string{"myorg", "myrepo", "main"},
	}

	if !Match(rule, res, VerbUpdate) {
		t.Error("glob path pattern should match")
	}

	resNoMatch := Resource{
		Path: []string{"myorg", "myrepo", "develop"},
	}
	if Match(rule, resNoMatch, VerbUpdate) {
		t.Error("glob path pattern should not match different branch")
	}
}

func TestMatchEnvironment(t *testing.T) {
	rule := ResourceRule{
		Environment: "prod",
		Decision:    "block",
	}
	devRes := Resource{Environment: "dev"}
	prodRes := Resource{Environment: "prod"}

	if Match(rule, devRes, VerbDelete) {
		t.Error("prod rule should not match dev resource")
	}
	if !Match(rule, prodRes, VerbDelete) {
		t.Error("prod rule should match prod resource")
	}
}

func TestMatchSensitivityThreshold(t *testing.T) {
	rule := ResourceRule{
		Sensitivity: "confidential",
		Decision:    "review",
	}

	publicRes := Resource{Sensitivity: SensitivityPublic}
	internalRes := Resource{Sensitivity: SensitivityInternal}
	confRes := Resource{Sensitivity: SensitivityConfidential}
	secretRes := Resource{Sensitivity: SensitivitySecret}

	if Match(rule, publicRes, VerbRead) {
		t.Error("public should not meet confidential threshold")
	}
	if Match(rule, internalRes, VerbRead) {
		t.Error("internal should not meet confidential threshold")
	}
	if !Match(rule, confRes, VerbRead) {
		t.Error("confidential should meet confidential threshold")
	}
	if !Match(rule, secretRes, VerbRead) {
		t.Error("secret should meet confidential threshold")
	}
}

func TestMatchVerbMismatch(t *testing.T) {
	rule := ResourceRule{
		Verb:     "delete",
		Decision: "block",
	}
	res := Resource{Type: ResourceTable}

	if Match(rule, res, VerbRead) {
		t.Error("delete rule should not match read verb")
	}
	if !Match(rule, res, VerbDelete) {
		t.Error("delete rule should match delete verb")
	}
}

func TestMatchProviderMismatch(t *testing.T) {
	rule := ResourceRule{
		Provider: "github",
		Decision: "allow",
	}
	res := Resource{Provider: "postgres"}

	if Match(rule, res, VerbRead) {
		t.Error("github rule should not match postgres resource")
	}
}
