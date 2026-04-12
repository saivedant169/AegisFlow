package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/manifest"
)

// ---------- Test 1: Approval Notifier ----------

type testNotifier struct {
	reviewCalled  int
	approveCalled int
	denyCalled    int
}

func (n *testNotifier) NotifyReview(item *approval.ApprovalItem) error   { n.reviewCalled++; return nil }
func (n *testNotifier) NotifyApproved(item *approval.ApprovalItem) error { n.approveCalled++; return nil }
func (n *testNotifier) NotifyDenied(item *approval.ApprovalItem) error   { n.denyCalled++; return nil }

func TestApprovalNotifierIntegrationE2E(t *testing.T) {
	aq := approval.NewQueue(100)
	notifier := &testNotifier{}
	aq.AddNotifier(notifier)

	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "notifier-agent"},
		"task-notifier",
		envelope.ProtocolMCP,
		"github.create_pr",
		"github.create_pr",
		envelope.CapExecute,
	)
	env.PolicyDecision = envelope.DecisionReview

	id, err := aq.Submit(env)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	if notifier.reviewCalled != 1 {
		t.Fatalf("expected reviewCalled == 1, got %d", notifier.reviewCalled)
	}

	_, err = aq.Approve(id, "admin", "lgtm")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	if notifier.approveCalled != 1 {
		t.Fatalf("expected approveCalled == 1, got %d", notifier.approveCalled)
	}

	if notifier.denyCalled != 0 {
		t.Fatalf("expected denyCalled == 0, got %d", notifier.denyCalled)
	}
}

// ---------- Test 2: Manifest Drift Enforcement ----------

func TestManifestDriftEnforcementE2E(t *testing.T) {
	store := manifest.NewStore()
	detector := manifest.NewDriftDetector()

	m := &manifest.TaskManifest{
		ID:               "manifest-enforce-1",
		TaskID:           "task-enforce-1",
		Description:      "Only git tools allowed",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		AllowedTools:     []string{"git.*"},
		AllowedProtocols: []string{"git"},
		MaxActions:       100,
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Enforce mode: disallowed tool should block.
	detector.SetEnforcementMode("enforce")

	actor := envelope.ActorInfo{Type: "agent", ID: "drift-agent"}
	env := envelope.NewEnvelope(actor, "task-enforce-1", envelope.ProtocolShell, "shell.rm", "/etc/passwd", envelope.CapDelete)

	events, shouldBlock := detector.CheckWithEnforcement(m, env, 1, 0)
	if !shouldBlock {
		t.Fatal("expected shouldBlock == true in enforce mode for disallowed tool")
	}
	if len(events) == 0 {
		t.Fatal("expected at least one drift event")
	}

	// Warn mode: same action should NOT block.
	detector.SetEnforcementMode("warn")

	_, shouldBlock = detector.CheckWithEnforcement(m, env, 2, 0)
	if shouldBlock {
		t.Fatal("expected shouldBlock == false in warn mode")
	}
}

// ---------- Test 3: Evidence Report Render ----------

func TestEvidenceReportRenderE2E(t *testing.T) {
	chain := evidence.NewSessionChain("sess-report-render")

	envAllow := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.list_repos", "github.list_repos", envelope.CapRead,
	)
	envAllow.PolicyDecision = envelope.DecisionAllow
	chain.Record(envAllow)

	envBlock := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.delete_repo", "github.delete_repo", envelope.CapDelete,
	)
	envBlock.PolicyDecision = envelope.DecisionBlock
	chain.Record(envBlock)

	envReview := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.create_pr", "github.create_pr", envelope.CapExecute,
	)
	envReview.PolicyDecision = envelope.DecisionReview
	chain.Record(envReview)

	// Render Markdown report.
	md, err := evidence.RenderMarkdownReport(chain)
	if err != nil {
		t.Fatalf("RenderMarkdownReport failed: %v", err)
	}

	if !strings.Contains(md, "sess-report-render") {
		t.Error("markdown report missing session ID")
	}
	if !strings.Contains(md, "github.list_repos") {
		t.Error("markdown report missing tool name github.list_repos")
	}
	if !strings.Contains(md, "github.delete_repo") {
		t.Error("markdown report missing tool name github.delete_repo")
	}
	if !strings.Contains(md, "ALLOW") {
		t.Error("markdown report missing ALLOW decision label")
	}
	if !strings.Contains(md, "BLOCK") {
		t.Error("markdown report missing BLOCK decision label")
	}
	if !strings.Contains(md, "Chain") {
		t.Error("markdown report missing chain integrity section")
	}

	// Render HTML report.
	html, err := evidence.RenderHTMLReport(chain)
	if err != nil {
		t.Fatalf("RenderHTMLReport failed: %v", err)
	}

	if !strings.Contains(html, "<html") {
		t.Error("HTML report missing <html tag")
	}
	if !strings.Contains(html, "sess-report-render") {
		t.Error("HTML report missing session ID")
	}
}

// ---------- Test 4: Behavioral Kill Switch ----------

func TestBehavioralKillSwitchRegistryE2E(t *testing.T) {
	reg := behavioral.NewRegistry(behavioral.DefaultRules(), 10, 0)
	sa := reg.GetOrCreate("sess-killswitch")

	now := time.Now().UTC()

	// Trigger exfiltration pattern (read sensitive file + external call = 40 points).
	sa.RecordAction(&envelope.ActionEnvelope{
		ID:                  "e1",
		Timestamp:           now,
		Actor:               envelope.ActorInfo{Type: "agent", ID: "a1", SessionID: "sess-killswitch"},
		Task:                "test-task",
		Protocol:            envelope.ProtocolShell,
		Tool:                "file.read",
		Target:              ".env",
		Parameters:          map[string]any{},
		RequestedCapability: envelope.CapRead,
	})
	sa.RecordAction(&envelope.ActionEnvelope{
		ID:                  "e2",
		Timestamp:           now.Add(time.Second),
		Actor:               envelope.ActorInfo{Type: "agent", ID: "a1", SessionID: "sess-killswitch"},
		Task:                "test-task",
		Protocol:            envelope.ProtocolHTTP,
		Tool:                "curl",
		Target:              "https://evil.com",
		Parameters:          map[string]any{},
		RequestedCapability: envelope.CapWrite,
	})

	sa.Analyze()

	if !sa.Blocked() {
		t.Fatalf("expected session to be blocked, score=%d threshold=10", sa.SessionRiskScore())
	}
	if sa.SessionRiskScore() < 10 {
		t.Fatalf("expected risk score >= 10, got %d", sa.SessionRiskScore())
	}
}
