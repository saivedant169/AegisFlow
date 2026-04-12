package evidence

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// RenderMarkdownReport generates a human-readable Markdown evidence report.
func RenderMarkdownReport(chain *SessionChain) (string, error) {
	manifest := chain.Manifest()
	records := chain.Records()
	verify := Verify(records)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Evidence Report: %s\n\n", manifest.SessionID))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	// Summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Actions | %d |\n", manifest.TotalActions))
	sb.WriteString(fmt.Sprintf("| Allowed | %d |\n", manifest.Allowed))
	sb.WriteString(fmt.Sprintf("| Reviewed | %d |\n", manifest.Reviewed))
	sb.WriteString(fmt.Sprintf("| Blocked | %d |\n", manifest.Blocked))
	sb.WriteString(fmt.Sprintf("| Chain Valid | %v |\n", verify.Valid))
	sb.WriteString(fmt.Sprintf("| First Hash | `%s` |\n", truncHash(manifest.FirstHash)))
	sb.WriteString(fmt.Sprintf("| Last Hash | `%s` |\n", truncHash(manifest.LastHash)))
	sb.WriteString("\n")

	if manifest.TotalActions == 0 {
		sb.WriteString("*No actions recorded.*\n")
		return sb.String(), nil
	}

	// Chain integrity
	sb.WriteString("## Chain Integrity\n\n")
	if verify.Valid {
		sb.WriteString("All record hashes verified. Chain is intact.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("**INTEGRITY FAILURE** at record %d: %s\n\n",
			verify.ErrorAtIndex, verify.Message))
	}

	// Action timeline
	sb.WriteString("## Action Timeline\n\n")
	sb.WriteString("| # | Time | Tool | Target | Decision | Hash |\n")
	sb.WriteString("|---|------|------|--------|----------|------|\n")
	for _, r := range records {
		decision := strings.ToUpper(string(r.Envelope.PolicyDecision))
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | `%s` |\n",
			r.Index,
			r.Timestamp.Format("15:04:05"),
			r.Envelope.Tool,
			truncTarget(r.Envelope.Target),
			decision,
			truncHash(r.Hash),
		))
	}

	return sb.String(), nil
}

// RenderHTMLReport generates a standalone HTML evidence report.
func RenderHTMLReport(chain *SessionChain) (string, error) {
	md, err := RenderMarkdownReport(chain)
	if err != nil {
		return "", err
	}

	manifest := chain.Manifest()

	const tmplStr = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Evidence Report: {{.SessionID}}</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 900px; margin: 40px auto; padding: 0 20px; color: #333; line-height: 1.6; }
pre { background: #f6f8fa; padding: 16px; border-radius: 6px; overflow-x: auto; white-space: pre-wrap; font-size: 14px; }
</style>
</head>
<body>
<pre>{{.Content}}</pre>
</body>
</html>`

	t, err := template.New("report").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]interface{}{
		"SessionID": manifest.SessionID,
		"Content":   md,
	}); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

func truncHash(h string) string {
	if len(h) > 12 {
		return h[:12] + "..."
	}
	return h
}

func truncTarget(t string) string {
	if len(t) > 40 {
		return t[:37] + "..."
	}
	return t
}
