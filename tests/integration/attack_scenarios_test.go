package integration

import (
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/githubgate"
	"github.com/saivedant169/AegisFlow/internal/shellgate"
	"github.com/saivedant169/AegisFlow/internal/sqlgate"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// ---------- Attack Scenario E2E Tests ----------

func TestAttackForkBombBlockedE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.*", Decision: "allow"},
	}, "allow")
	ev := evidence.NewSessionChain("sess-attack-fork")
	aq := approval.NewQueue(100)

	interceptor := shellgate.NewInterceptor(pe, ev, aq, true)

	// Fork bomb: :(){ :|:& };:
	result, err := interceptor.Evaluate("bash", []string{"-c", ":(){ :|:& };:"}, "/tmp")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionBlock {
		t.Fatalf("expected block for fork bomb, got %s", result.Decision)
	}
	if !strings.Contains(result.Message, "dangerous") {
		t.Errorf("expected 'dangerous' in message, got %q", result.Message)
	}

	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected block in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestAttackCredentialTheftBlockedE2E(t *testing.T) {
	// Block credential theft commands via policy rules targeting specific tools.
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.cat", Decision: "block"},
		{Protocol: "shell", Tool: "shell.printenv", Decision: "block"},
		{Protocol: "shell", Tool: "shell.env", Decision: "block"},
	}, "allow")
	ev := evidence.NewSessionChain("sess-attack-cred-theft")
	aq := approval.NewQueue(100)

	interceptor := shellgate.NewInterceptor(pe, ev, aq, true)

	attacks := []struct {
		name string
		cmd  string
		args []string
	}{
		{"cat /etc/shadow", "cat", []string{"/etc/shadow"}},
		{"cat .env", "cat", []string{".env"}},
		{"printenv", "printenv", nil},
	}

	for _, attack := range attacks {
		t.Run(attack.name, func(t *testing.T) {
			result, err := interceptor.Evaluate(attack.cmd, attack.args, "/tmp")
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}
			if result.Decision != envelope.DecisionBlock {
				t.Errorf("expected block for %q, got %s", attack.name, result.Decision)
			}
		})
	}

	// All three should be in the evidence chain.
	records := ev.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 evidence records, got %d", len(records))
	}
	for _, rec := range records {
		if rec.Envelope.PolicyDecision != envelope.DecisionBlock {
			t.Errorf("expected block decision for %s, got %s", rec.Envelope.Tool, rec.Envelope.PolicyDecision)
		}
	}
}

func TestAttackSQLInjectionBlockedE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "sql.select", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-attack-sqli")

	interceptor := sqlgate.NewInterceptor(pe, ev, true)

	injections := []struct {
		name  string
		query string
	}{
		{"DROP TABLE injection", "DROP TABLE users; --"},
		{"TRUNCATE injection", "TRUNCATE TABLE sessions"},
		{"DELETE without WHERE", "DELETE FROM users"},
	}

	for _, inj := range injections {
		t.Run(inj.name, func(t *testing.T) {
			result, err := interceptor.Evaluate(inj.query, "production")
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}
			if result.Decision != envelope.DecisionBlock {
				t.Errorf("expected block for %q, got %s", inj.name, result.Decision)
			}
		})
	}

	records := ev.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 evidence records, got %d", len(records))
	}
	for _, rec := range records {
		if rec.Envelope.PolicyDecision != envelope.DecisionBlock {
			t.Errorf("expected block for %s, got %s", rec.Envelope.Tool, rec.Envelope.PolicyDecision)
		}
	}
}

func TestAttackOverScopedGitHubBlockedE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.get_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
		{Protocol: "git", Tool: "github.push", Decision: "block"},
	}, "block")
	ev := evidence.NewSessionChain("sess-attack-gh-scope")
	actor := envelope.ActorInfo{Type: "agent", ID: "attacker-agent"}

	interceptor := githubgate.NewInterceptor(pe, ev, actor)

	attacks := []struct {
		name      string
		operation string
		repo      string
	}{
		{"delete_repo", "delete_repo", "myorg/production-app"},
		{"push (force push)", "push", "myorg/production-app"},
	}

	for _, attack := range attacks {
		t.Run(attack.name, func(t *testing.T) {
			result, err := interceptor.Evaluate(attack.operation, attack.repo, map[string]any{"force": true})
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}
			if result.Decision != envelope.DecisionBlock {
				t.Errorf("expected block for %q, got %s", attack.name, result.Decision)
			}
		})
	}

	records := ev.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 evidence records, got %d", len(records))
	}
	for _, rec := range records {
		if rec.Envelope.PolicyDecision != envelope.DecisionBlock {
			t.Errorf("expected block for %s, got %s", rec.Envelope.Tool, rec.Envelope.PolicyDecision)
		}
	}
}

func TestFullAttackSuiteE2E(t *testing.T) {
	// Shared evidence chain for all attacks.
	ev := evidence.NewSessionChain("sess-full-attack-suite")

	// Shell interceptor setup.
	shellPE := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.ls", Decision: "allow"},
		{Protocol: "shell", Tool: "shell.cat", Decision: "block"},
		{Protocol: "shell", Tool: "shell.printenv", Decision: "block"},
	}, "block")
	shellAQ := approval.NewQueue(100)
	shellInterceptor := shellgate.NewInterceptor(shellPE, ev, shellAQ, true)

	// SQL interceptor setup.
	sqlPE := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "sql.select", Decision: "allow"},
	}, "block")
	sqlInterceptor := sqlgate.NewInterceptor(sqlPE, ev, true)

	// GitHub interceptor setup.
	ghPE := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
		{Protocol: "git", Tool: "github.push", Decision: "block"},
	}, "block")
	ghActor := envelope.ActorInfo{Type: "agent", ID: "attack-suite-agent"}
	ghInterceptor := githubgate.NewInterceptor(ghPE, ev, ghActor)

	// 10 attacks that should all be blocked.
	type attack struct {
		name string
		fn   func() (envelope.Decision, error)
	}

	attacks := []attack{
		// 1. Fork bomb
		{"fork_bomb", func() (envelope.Decision, error) {
			r, err := shellInterceptor.Evaluate("bash", []string{"-c", ":(){ :|:& };:"}, "/tmp")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 2. rm -rf /
		{"rm_rf_root", func() (envelope.Decision, error) {
			r, err := shellInterceptor.Evaluate("rm", []string{"-rf", "/"}, "/")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 3. cat /etc/shadow
		{"cat_shadow", func() (envelope.Decision, error) {
			r, err := shellInterceptor.Evaluate("cat", []string{"/etc/shadow"}, "/tmp")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 4. printenv
		{"printenv_leak", func() (envelope.Decision, error) {
			r, err := shellInterceptor.Evaluate("printenv", nil, "/tmp")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 5. DROP TABLE
		{"sql_drop_table", func() (envelope.Decision, error) {
			r, err := sqlInterceptor.Evaluate("DROP TABLE users", "production")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 6. TRUNCATE TABLE
		{"sql_truncate", func() (envelope.Decision, error) {
			r, err := sqlInterceptor.Evaluate("TRUNCATE TABLE sessions", "production")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 7. DELETE without WHERE
		{"sql_delete_no_where", func() (envelope.Decision, error) {
			r, err := sqlInterceptor.Evaluate("DELETE FROM users", "production")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 8. GitHub delete repo
		{"gh_delete_repo", func() (envelope.Decision, error) {
			r, err := ghInterceptor.Evaluate("delete_repo", "myorg/prod", map[string]any{})
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 9. GitHub force push
		{"gh_force_push", func() (envelope.Decision, error) {
			r, err := ghInterceptor.Evaluate("push", "myorg/prod", map[string]any{"force": true})
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
		// 10. chmod 777
		{"chmod_777", func() (envelope.Decision, error) {
			r, err := shellInterceptor.Evaluate("chmod", []string{"777", "/etc/passwd"}, "/")
			if err != nil {
				return "", err
			}
			return r.Decision, nil
		}},
	}

	for _, atk := range attacks {
		t.Run(atk.name, func(t *testing.T) {
			decision, err := atk.fn()
			if err != nil {
				t.Fatalf("attack %q failed: %v", atk.name, err)
			}
			if decision != envelope.DecisionBlock {
				t.Errorf("attack %q: expected block, got %s", atk.name, decision)
			}
		})
	}

	// Verify evidence chain integrity.
	records := ev.Records()
	if len(records) != 10 {
		t.Fatalf("expected 10 evidence records (one per attack), got %d", len(records))
	}

	// All records must be blocks.
	for i, rec := range records {
		if rec.Envelope.PolicyDecision != envelope.DecisionBlock {
			t.Errorf("record %d: expected block, got %s (tool=%s)",
				i, rec.Envelope.PolicyDecision, rec.Envelope.Tool)
		}
	}

	// Verify chain integrity with hash linking.
	result := evidence.Verify(records)
	if !result.Valid {
		t.Fatalf("evidence chain verification failed: %s (error at index %d)", result.Message, result.ErrorAtIndex)
	}
	if result.TotalRecords != 10 {
		t.Errorf("expected 10 total records in verification, got %d", result.TotalRecords)
	}
}
