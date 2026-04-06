package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
)

// testActionResult holds the response from either the admin API or local evaluation.
type testActionResult struct {
	Decision     string `json:"decision"`
	EnvelopeID   string `json:"envelope_id"`
	EvidenceHash string `json:"evidence_hash"`
	Message      string `json:"message"`
	ApprovalID   string `json:"approval_id,omitempty"`
	Local        bool   `json:"-"`
}

func cmdTestAction(adminURL string, args []string) {
	var protocol, tool, target, capability, paramsStr string

	// Parse flags manually to match the existing CLI pattern.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--protocol":
			if i+1 < len(args) {
				protocol = args[i+1]
				i++
			}
		case "--tool":
			if i+1 < len(args) {
				tool = args[i+1]
				i++
			}
		case "--target":
			if i+1 < len(args) {
				target = args[i+1]
				i++
			}
		case "--capability":
			if i+1 < len(args) {
				capability = args[i+1]
				i++
			}
		case "--params":
			if i+1 < len(args) {
				paramsStr = args[i+1]
				i++
			}
		}
	}

	if protocol == "" || tool == "" || target == "" {
		fmt.Println("Usage: aegisctl test-action --protocol <mcp|shell|sql|git|http> --tool <tool_name> --target <target> [--capability <read|write|delete|deploy>] [--params key=value,...]")
		os.Exit(1)
	}

	// Parse params
	params := make(map[string]string)
	if paramsStr != "" {
		for _, pair := range strings.Split(paramsStr, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				params[kv[0]] = kv[1]
			}
		}
	}

	// Try remote evaluation first
	result, err := remoteTestAction(adminURL, protocol, tool, target, capability, params)
	if err != nil {
		// Fall back to local evaluation
		result = localTestAction(protocol, tool, target, capability, params)
	}

	printTestActionResult(result)
}

func remoteTestAction(adminURL, protocol, tool, target, capability string, params map[string]string) (*testActionResult, error) {
	body := map[string]interface{}{
		"protocol":   protocol,
		"tool":       tool,
		"target":     target,
		"capability": capability,
		"params":     params,
	}
	data, _ := json.Marshal(body)

	resp, err := client.Post(adminURL+"/admin/v1/test-action", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result testActionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func localTestAction(protocol, tool, target, capability string, params map[string]string) *testActionResult {
	// Build default rules that demonstrate the governance pipeline.
	rules := []toolpolicy.ToolRule{
		// Block dangerous shell commands
		{Protocol: "shell", Tool: "rm", Decision: "block"},
		{Protocol: "shell", Tool: "shutdown", Decision: "block"},
		{Protocol: "shell", Tool: "reboot", Decision: "block"},
		{Protocol: "shell", Tool: "mkfs", Decision: "block"},
		// Block dangerous SQL operations
		{Protocol: "sql", Tool: "*", Capability: "delete", Decision: "review"},
		{Protocol: "sql", Tool: "drop_*", Decision: "block"},
		// Git write operations need review
		{Protocol: "git", Tool: "delete_*", Decision: "block"},
		{Protocol: "git", Tool: "merge_*", Capability: "deploy", Decision: "review"},
		{Protocol: "git", Tool: "push", Decision: "review"},
		// Read operations are generally allowed
		{Protocol: "*", Tool: "list_*", Capability: "read", Decision: "allow"},
		{Protocol: "*", Tool: "get_*", Capability: "read", Decision: "allow"},
		// Default: review everything else
	}

	engine := toolpolicy.NewEngine(rules, "review")

	cap := envelope.Capability(capability)
	if cap == "" {
		cap = envelope.CapExecute
	}

	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "aegisctl-local",
		SessionID: "local-session",
		TenantID:  "local-tenant",
	}
	env := envelope.NewEnvelope(actor, "test-action", envelope.Protocol(protocol), tool, target, cap)
	for k, v := range params {
		env.Parameters[k] = v
	}

	decision := engine.Evaluate(env)
	env.PolicyDecision = decision
	env.EvidenceHash = env.Hash()

	result := &testActionResult{
		Decision:     string(decision),
		EnvelopeID:   env.ID,
		EvidenceHash: env.EvidenceHash,
		Local:        true,
	}

	switch decision {
	case envelope.DecisionAllow:
		result.Message = "Action is allowed by policy"
	case envelope.DecisionReview:
		result.Message = "Action requires human review"
		result.ApprovalID = env.ID
	case envelope.DecisionBlock:
		result.Message = "Action is blocked by policy"
	}

	return result
}

func formatTestActionOutput(result *testActionResult) string {
	var sb strings.Builder

	if result.Local {
		sb.WriteString("(local evaluation - admin server not reachable)\n\n")
	}

	// Decision with color
	switch result.Decision {
	case "allow":
		sb.WriteString(fmt.Sprintf("Decision:      %sALLOWED%s\n", colorGreen, colorReset))
	case "review":
		sb.WriteString(fmt.Sprintf("Decision:      %sREVIEW REQUIRED%s\n", colorYellow, colorReset))
	case "block":
		sb.WriteString(fmt.Sprintf("Decision:      %sBLOCKED%s\n", colorRed, colorReset))
	default:
		sb.WriteString(fmt.Sprintf("Decision:      %s\n", result.Decision))
	}

	sb.WriteString(fmt.Sprintf("Envelope ID:   %s\n", result.EnvelopeID))
	sb.WriteString(fmt.Sprintf("Evidence Hash: %s\n", result.EvidenceHash))
	sb.WriteString(fmt.Sprintf("Message:       %s\n", result.Message))

	if result.ApprovalID != "" {
		sb.WriteString(fmt.Sprintf("Approval ID:   %s\n", result.ApprovalID))
	}

	return sb.String()
}

func printTestActionResult(result *testActionResult) {
	fmt.Print(formatTestActionOutput(result))
}
