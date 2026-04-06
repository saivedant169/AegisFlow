package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
)

// GitHubNotifier posts approval lifecycle comments on GitHub PRs/issues.
type GitHubNotifier struct {
	token   string
	baseURL string // https://api.github.com or injectable for tests
	repo    string // owner/repo
	client  *http.Client
}

// NewGitHubNotifier creates a notifier that posts comments via the GitHub API.
// baseURL should be "https://api.github.com" in production; repo is "owner/repo".
func NewGitHubNotifier(token, baseURL, repo string) *GitHubNotifier {
	return &GitHubNotifier{
		token:   token,
		baseURL: baseURL,
		repo:    repo,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyReview posts a comment on the associated PR/issue describing the
// pending approval: tool, target, capability, risk level, evidence hash,
// and deep links back to the admin API for approve/deny.
func (g *GitHubNotifier) NotifyReview(item *approval.ApprovalItem) error {
	risk := riskLevel(item)
	body := fmt.Sprintf(
		"## :shield: AegisFlow Approval Required\n\n"+
			"| Field | Value |\n"+
			"|-------|-------|\n"+
			"| **Tool** | `%s` |\n"+
			"| **Target** | `%s` |\n"+
			"| **Capability** | `%s` |\n"+
			"| **Risk** | %s |\n"+
			"| **Evidence Hash** | `%s` |\n"+
			"| **Approval ID** | `%s` |\n\n"+
			"**Approve:** `POST /admin/approvals/%s/approve`\n"+
			"**Deny:** `POST /admin/approvals/%s/deny`\n",
		item.Envelope.Tool,
		item.Envelope.Target,
		string(item.Envelope.RequestedCapability),
		risk,
		item.Envelope.EvidenceHash,
		item.ID,
		item.ID,
		item.ID,
	)
	return g.postComment(item, body)
}

// NotifyApproved posts a follow-up comment indicating approval.
func (g *GitHubNotifier) NotifyApproved(item *approval.ApprovalItem) error {
	body := fmt.Sprintf(
		":white_check_mark: **Approved** by %s: %s\n\nApproval ID: `%s`",
		item.Reviewer, item.ReviewComment, item.ID,
	)
	return g.postComment(item, body)
}

// NotifyDenied posts a follow-up comment indicating denial.
func (g *GitHubNotifier) NotifyDenied(item *approval.ApprovalItem) error {
	body := fmt.Sprintf(
		":x: **Denied** by %s: %s\n\nApproval ID: `%s`",
		item.Reviewer, item.ReviewComment, item.ID,
	)
	return g.postComment(item, body)
}

func (g *GitHubNotifier) postComment(item *approval.ApprovalItem, body string) error {
	// Derive issue/PR number from the envelope target or task.
	// Convention: item.Envelope.Target contains "PR#<number>" or we fall back
	// to posting on issue #1. Integrators should set Target accordingly.
	issueNumber := extractIssueNumber(item)

	url := fmt.Sprintf("%s/repos/%s/issues/%s/comments", g.baseURL, g.repo, issueNumber)

	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return fmt.Errorf("github: marshal comment: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("github: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("github: post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("github: API returned status %d", resp.StatusCode)
	}
	return nil
}

// extractIssueNumber pulls a PR/issue number from the envelope.
// It looks for a "pr_number" parameter first, then falls back to "1".
func extractIssueNumber(item *approval.ApprovalItem) string {
	if item.Envelope != nil && item.Envelope.Parameters != nil {
		if pr, ok := item.Envelope.Parameters["pr_number"]; ok {
			return fmt.Sprintf("%v", pr)
		}
	}
	return "1"
}

// riskLevel derives a human-readable risk string from the envelope.
func riskLevel(item *approval.ApprovalItem) string {
	if item.Envelope == nil {
		return "unknown"
	}
	if item.Envelope.IsDestructive() {
		return ":red_circle: HIGH"
	}
	return ":yellow_circle: MEDIUM"
}
