package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
)

// SlackNotifier sends approval lifecycle messages to a Slack webhook.
type SlackNotifier struct {
	webhookURL string
	adminURL   string // base URL for approve/deny deep links
	client     *http.Client
}

// NewSlackNotifier creates a notifier that posts Block Kit messages to Slack.
func NewSlackNotifier(webhookURL, adminURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		adminURL:   adminURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// slackPayload is a minimal Slack Block Kit message structure.
type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type     string        `json:"type"`
	Text     *slackText    `json:"text,omitempty"`
	Elements []slackButton `json:"elements,omitempty"`
	Fields   []slackText   `json:"fields,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackButton struct {
	Type string    `json:"type"`
	Text slackText `json:"text"`
	URL  string    `json:"url"`
}

// NotifyReview sends a Block Kit formatted message with action details,
// approve/deny buttons, evidence hash, and urgency indicator.
func (s *SlackNotifier) NotifyReview(item *approval.ApprovalItem) error {
	urgency := ":large_yellow_circle: Review Needed"
	if item.Envelope != nil && item.Envelope.IsDestructive() {
		urgency = ":red_circle: URGENT Review Needed"
	}

	evidence := ""
	if item.Envelope != nil {
		evidence = item.Envelope.EvidenceHash
	}

	tool, target, cap := "", "", ""
	if item.Envelope != nil {
		tool = item.Envelope.Tool
		target = item.Envelope.Target
		cap = string(item.Envelope.RequestedCapability)
	}

	payload := slackPayload{
		Blocks: []slackBlock{
			{
				Type: "header",
				Text: &slackText{Type: "plain_text", Text: urgency},
			},
			{
				Type: "section",
				Fields: []slackText{
					{Type: "mrkdwn", Text: fmt.Sprintf("*Tool:*\n`%s`", tool)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Target:*\n`%s`", target)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Capability:*\n`%s`", cap)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Evidence Hash:*\n`%s`", evidence)},
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Approval ID:* `%s`", item.ID),
				},
			},
			{
				Type: "actions",
				Elements: []slackButton{
					{
						Type: "button",
						Text: slackText{Type: "plain_text", Text: "Approve"},
						URL:  fmt.Sprintf("%s/admin/approvals/%s/approve", s.adminURL, item.ID),
					},
					{
						Type: "button",
						Text: slackText{Type: "plain_text", Text: "Deny"},
						URL:  fmt.Sprintf("%s/admin/approvals/%s/deny", s.adminURL, item.ID),
					},
				},
			},
		},
	}

	return s.send(payload)
}

// NotifyApproved posts a follow-up message that the item was approved.
func (s *SlackNotifier) NotifyApproved(item *approval.ApprovalItem) error {
	payload := slackPayload{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":white_check_mark: *Approved* by %s: %s\nApproval ID: `%s`",
						item.Reviewer, item.ReviewComment, item.ID),
				},
			},
		},
	}
	return s.send(payload)
}

// NotifyDenied posts a follow-up message that the item was denied.
func (s *SlackNotifier) NotifyDenied(item *approval.ApprovalItem) error {
	payload := slackPayload{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":x: *Denied* by %s: %s\nApproval ID: `%s`",
						item.Reviewer, item.ReviewComment, item.ID),
				},
			},
		},
	}
	return s.send(payload)
}

func (s *SlackNotifier) send(payload slackPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack: webhook returned status %d", resp.StatusCode)
	}
	return nil
}
