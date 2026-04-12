package manifest

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func makeTestEnvelope(protocol, tool, target string, capability envelope.Capability) *envelope.ActionEnvelope {
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "test-agent",
		SessionID: "test-session",
		TenantID:  "test-tenant",
	}
	return envelope.NewEnvelope(actor, "test-task", envelope.Protocol(protocol), tool, target, capability)
}

func makeTestManifest() *TaskManifest {
	return &TaskManifest{
		ID:               "manifest-1",
		TaskID:           "TICKET-123",
		Description:      "Deploy service update",
		Owner:            "alice",
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        time.Now().UTC().Add(1 * time.Hour),
		AllowedTools:     []string{"github.*", "git_push"},
		AllowedResources: []string{"repos/myorg/*", "repos/myorg/myrepo"},
		AllowedProtocols: []string{"git", "shell"},
		AllowedVerbs:     []string{"read", "write"},
		MaxActions:       50,
		MaxBudget:        100.0,
		RiskTier:         "medium",
		Active:           true,
	}
}

func TestNoDriftWhenWithinScope(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("git", "github.list_repos", "repos/myorg/myrepo", envelope.CapRead)

	events := d.Check(m, env, 1, 0.0)
	if len(events) != 0 {
		t.Errorf("expected 0 drift events, got %d: %+v", len(events), events)
	}
}

func TestDriftUnexpectedTool(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("git", "slack.post_message", "repos/myorg/myrepo", envelope.CapRead)

	events := d.Check(m, env, 1, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftUnexpectedTool {
			found = true
			if e.Severity != "violation" {
				t.Errorf("expected severity violation, got %s", e.Severity)
			}
		}
	}
	if !found {
		t.Error("expected DriftUnexpectedTool event")
	}
}

func TestDriftUnexpectedResource(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("git", "github.delete_repo", "repos/other-org/secret", envelope.CapRead)

	events := d.Check(m, env, 1, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftUnexpectedResource {
			found = true
		}
	}
	if !found {
		t.Error("expected DriftUnexpectedResource event")
	}
}

func TestDriftUnexpectedProtocol(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("sql", "github.list_repos", "repos/myorg/myrepo", envelope.CapRead)

	events := d.Check(m, env, 1, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftUnexpectedProtocol {
			found = true
		}
	}
	if !found {
		t.Error("expected DriftUnexpectedProtocol event")
	}
}

func TestDriftUnexpectedVerb(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("git", "github.delete_repo", "repos/myorg/myrepo", envelope.CapDelete)

	events := d.Check(m, env, 1, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftUnexpectedVerb {
			found = true
			if e.Severity != "warning" {
				t.Errorf("expected severity warning, got %s", e.Severity)
			}
		}
	}
	if !found {
		t.Error("expected DriftUnexpectedVerb event")
	}
}

func TestDriftExceededActions(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	env := makeTestEnvelope("git", "github.list_repos", "repos/myorg/myrepo", envelope.CapRead)

	events := d.Check(m, env, 51, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftExceededActions {
			found = true
		}
	}
	if !found {
		t.Error("expected DriftExceededActions event")
	}
}

func TestDriftExpiredManifest(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	m.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour) // expired
	env := makeTestEnvelope("git", "github.list_repos", "repos/myorg/myrepo", envelope.CapRead)

	events := d.Check(m, env, 1, 0.0)
	found := false
	for _, e := range events {
		if e.Type == DriftExpiredManifest {
			found = true
		}
	}
	if !found {
		t.Error("expected DriftExpiredManifest event")
	}
}

func TestMultipleDriftsInOneAction(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()
	// Use wrong protocol, wrong tool, wrong resource, wrong verb, and exceed actions
	env := makeTestEnvelope("http", "slack.post", "external/webhook", envelope.CapDeploy)

	events := d.Check(m, env, 100, 200.0)
	// Should have: unexpected_tool, unexpected_resource, unexpected_protocol, unexpected_verb, exceeded_max_actions, exceeded_max_budget
	types := make(map[DriftType]bool)
	for _, e := range events {
		types[e.Type] = true
	}

	expected := []DriftType{
		DriftUnexpectedTool,
		DriftUnexpectedResource,
		DriftUnexpectedProtocol,
		DriftUnexpectedVerb,
		DriftExceededActions,
		DriftExceededBudget,
	}
	for _, dt := range expected {
		if !types[dt] {
			t.Errorf("expected drift type %s not found in events", dt)
		}
	}
}

func TestGlobMatchingInManifest(t *testing.T) {
	d := NewDriftDetector()
	m := makeTestManifest()

	// github.list_repos matches "github.*"
	env1 := makeTestEnvelope("git", "github.list_repos", "repos/myorg/myrepo", envelope.CapRead)
	events1 := d.Check(m, env1, 1, 0.0)
	for _, e := range events1 {
		if e.Type == DriftUnexpectedTool {
			t.Error("github.list_repos should match github.* glob")
		}
	}

	// git_push matches "git_push" exactly
	env2 := makeTestEnvelope("git", "git_push", "repos/myorg/myrepo", envelope.CapWrite)
	events2 := d.Check(m, env2, 1, 0.0)
	for _, e := range events2 {
		if e.Type == DriftUnexpectedTool {
			t.Error("git_push should match git_push exactly")
		}
	}

	// repos/myorg/whatever should match repos/myorg/*
	env3 := makeTestEnvelope("git", "github.clone", "repos/myorg/whatever", envelope.CapRead)
	events3 := d.Check(m, env3, 1, 0.0)
	for _, e := range events3 {
		if e.Type == DriftUnexpectedResource {
			t.Error("repos/myorg/whatever should match repos/myorg/* glob")
		}
	}

	// repos/other/whatever should NOT match
	env4 := makeTestEnvelope("git", "github.clone", "repos/other/whatever", envelope.CapRead)
	events4 := d.Check(m, env4, 1, 0.0)
	found := false
	for _, e := range events4 {
		if e.Type == DriftUnexpectedResource {
			found = true
		}
	}
	if !found {
		t.Error("repos/other/whatever should NOT match repos/myorg/*")
	}
}

func TestCheckWithEnforcementEnforceBlocks(t *testing.T) {
	d := NewDriftDetector()
	d.SetEnforcementMode("enforce")
	m := makeTestManifest()
	// "shell.rm" does not match allowed tools "github.*" or "git_push"
	env := makeTestEnvelope("git", "shell.rm", "repos/myorg/myrepo", envelope.CapRead)

	events, shouldBlock := d.CheckWithEnforcement(m, env, 1, 0.0)
	if len(events) == 0 {
		t.Error("expected drift events for disallowed tool")
	}
	if !shouldBlock {
		t.Error("expected shouldBlock == true in enforce mode with violations")
	}
}

func TestCheckWithEnforcementWarnDoesNotBlock(t *testing.T) {
	d := NewDriftDetector()
	d.SetEnforcementMode("warn")
	m := makeTestManifest()
	// "shell.rm" does not match allowed tools "github.*" or "git_push"
	env := makeTestEnvelope("git", "shell.rm", "repos/myorg/myrepo", envelope.CapRead)

	events, shouldBlock := d.CheckWithEnforcement(m, env, 1, 0.0)
	if len(events) == 0 {
		t.Error("expected drift events for disallowed tool")
	}
	if shouldBlock {
		t.Error("expected shouldBlock == false in warn mode")
	}
}
