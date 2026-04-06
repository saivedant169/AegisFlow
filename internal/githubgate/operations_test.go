package githubgate

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestClassifyReadOps(t *testing.T) {
	readOps := []string{
		"list_repos", "get_repo", "list_pull_requests", "get_pull_request",
		"list_issues", "get_issue", "list_branches", "get_commit",
		"list_comments", "search_code",
	}
	for _, op := range readOps {
		cap, risk := ClassifyOperation(op)
		if cap != envelope.CapRead {
			t.Errorf("ClassifyOperation(%q) capability = %q, want %q", op, cap, envelope.CapRead)
		}
		if risk != RiskLow {
			t.Errorf("ClassifyOperation(%q) risk = %q, want %q", op, risk, RiskLow)
		}
	}
}

func TestClassifyWriteOps(t *testing.T) {
	writeOps := []string{
		"create_issue", "create_pull_request", "create_branch",
		"create_comment", "update_issue", "update_pull_request", "create_release",
	}
	for _, op := range writeOps {
		cap, risk := ClassifyOperation(op)
		if cap != envelope.CapWrite {
			t.Errorf("ClassifyOperation(%q) capability = %q, want %q", op, cap, envelope.CapWrite)
		}
		if risk != RiskMedium {
			t.Errorf("ClassifyOperation(%q) risk = %q, want %q", op, risk, RiskMedium)
		}
	}
}

func TestClassifyDeployOps(t *testing.T) {
	deployOps := []string{
		"merge_pull_request", "create_deployment", "push",
	}
	for _, op := range deployOps {
		cap, risk := ClassifyOperation(op)
		if cap != envelope.CapDeploy {
			t.Errorf("ClassifyOperation(%q) capability = %q, want %q", op, cap, envelope.CapDeploy)
		}
		if risk != RiskHigh {
			t.Errorf("ClassifyOperation(%q) risk = %q, want %q", op, risk, RiskHigh)
		}
	}
}

func TestClassifyDeleteOps(t *testing.T) {
	deleteOps := []string{
		"delete_repo", "delete_branch", "delete_release", "delete_comment",
	}
	for _, op := range deleteOps {
		cap, risk := ClassifyOperation(op)
		if cap != envelope.CapDelete {
			t.Errorf("ClassifyOperation(%q) capability = %q, want %q", op, cap, envelope.CapDelete)
		}
		if risk != RiskCritical {
			t.Errorf("ClassifyOperation(%q) risk = %q, want %q", op, risk, RiskCritical)
		}
	}
}

func TestClassifyUnknownOp(t *testing.T) {
	cap, risk := ClassifyOperation("some_unknown_operation")
	if cap != envelope.CapWrite {
		t.Errorf("ClassifyOperation(unknown) capability = %q, want %q", cap, envelope.CapWrite)
	}
	if risk != RiskMedium {
		t.Errorf("ClassifyOperation(unknown) risk = %q, want %q", risk, RiskMedium)
	}
}
