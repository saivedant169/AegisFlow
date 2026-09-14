package evidence

import (
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestRenderMarkdownReport(t *testing.T) {
	chain := NewSessionChain("report-test")
	_, err := chain.Record(testEnv("github.list_repos", envelope.DecisionAllow))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("github.create_pr", envelope.DecisionAllow))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("shell.rm", envelope.DecisionBlock))
	if err != nil {
		return
	}

	report, err := RenderMarkdownReport(chain)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if !strings.Contains(report, "report-test") {
		t.Error("report should contain session ID")
	}
	if !strings.Contains(report, "github.list_repos") {
		t.Error("report should contain tool names in timeline")
	}
	if !strings.Contains(report, "BLOCK") {
		t.Error("report should show blocked decisions")
	}
	if !strings.Contains(report, "Chain Integrity") {
		t.Error("report should have chain integrity section")
	}
}

func TestRenderMarkdownReportEmpty(t *testing.T) {
	chain := NewSessionChain("empty-session")
	report, err := RenderMarkdownReport(chain)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(report, "empty-session") {
		t.Error("empty report should contain session ID")
	}
	if !strings.Contains(report, "No actions recorded") {
		t.Error("empty report should indicate no actions")
	}
}

func TestRenderHTMLReport(t *testing.T) {
	chain := NewSessionChain("html-test")
	_, err := chain.Record(testEnv("github.list_repos", envelope.DecisionAllow))
	if err != nil {
		return
	}

	report, err := RenderHTMLReport(chain)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(report, "<html") {
		t.Error("HTML report should contain <html tag")
	}
	if !strings.Contains(report, "html-test") {
		t.Error("HTML report should contain session ID")
	}
	if !strings.Contains(report, "github.list_repos") {
		t.Error("HTML report should contain tool names")
	}
}
