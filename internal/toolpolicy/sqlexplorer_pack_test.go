package toolpolicy

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// TestSQLExplorerPack loads the shipped sql-explorer policy pack and asserts
// the decisions promised in its header and in issue #87, so the pack cannot
// silently drift from its documented behavior.
func TestSQLExplorerPack(t *testing.T) {
	data, err := os.ReadFile("../../starter-kit/policies/sql-explorer.yaml")
	if err != nil {
		t.Fatalf("read pack: %v", err)
	}
	var doc struct {
		ToolPolicies struct {
			Enabled         bool       `yaml:"enabled"`
			DefaultDecision string     `yaml:"default_decision"`
			Rules           []ToolRule `yaml:"rules"`
		} `yaml:"tool_policies"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse pack: %v", err)
	}
	if doc.ToolPolicies.DefaultDecision != "block" {
		t.Fatalf("default_decision = %q, want block", doc.ToolPolicies.DefaultDecision)
	}

	eng := NewEngine(doc.ToolPolicies.Rules, doc.ToolPolicies.DefaultDecision)

	eval := func(protocol, tool, target string) envelope.Decision {
		env := envelope.NewEnvelope(
			envelope.ActorInfo{Type: "agent", ID: "test"},
			"pack-test", envelope.Protocol(protocol), tool, target, envelope.CapExecute,
		)
		return eng.Evaluate(env)
	}

	cases := []struct {
		name     string
		protocol string
		tool     string
		target   string
		want     envelope.Decision
	}{
		{"select allowed", "sql", "sql.select", "prod", envelope.DecisionAllow},
		{"insert reviewed", "sql", "sql.insert", "prod", envelope.DecisionReview},
		{"update reviewed", "sql", "sql.update", "prod", envelope.DecisionReview},
		{"delete blocked", "sql", "sql.delete", "prod", envelope.DecisionBlock},
		{"drop_table blocked (glob)", "sql", "sql.drop_table", "prod", envelope.DecisionBlock},
		{"truncate blocked", "sql", "sql.truncate", "prod", envelope.DecisionBlock},
		{"grant blocked", "sql", "sql.grant", "prod", envelope.DecisionBlock},
		{"revoke blocked", "sql", "sql.revoke", "prod", envelope.DecisionBlock},
		{"github list allowed (git)", "git", "github.list_repos", "org/repo", envelope.DecisionAllow},
		{"github get allowed (mcp)", "mcp", "github.get_file", "org/repo", envelope.DecisionAllow},
		{"github write blocked", "git", "github.create_pull_request", "org/repo", envelope.DecisionBlock},
		{"shell read allowed", "shell", "shell.cat", "schema.sql", envelope.DecisionAllow},
		{"shell cat .env blocked", "shell", "shell.cat", ".env", envelope.DecisionBlock},
		{"shell write blocked", "shell", "shell.rm", "/data", envelope.DecisionBlock},
		{"http blocked by default", "http", "http.post", "example.com", envelope.DecisionBlock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := eval(c.protocol, c.tool, c.target); got != c.want {
				t.Fatalf("%s/%s -> %s, want %s", c.protocol, c.tool, got, c.want)
			}
		})
	}
}
