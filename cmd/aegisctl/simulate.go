package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// --- aegisctl simulate ---

func cmdSimulate(adminURL string, args []string) {
	var protocol, tool, target, capability string

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
		}
	}

	if protocol == "" || tool == "" || target == "" {
		fmt.Println("Usage: aegisctl simulate --protocol <protocol> --tool <tool> --target <target> [--capability <cap>]")
		os.Exit(1)
	}

	// Try remote first.
	result, err := remoteSimulate(adminURL, protocol, tool, target, capability)
	if err != nil {
		// Fall back to local.
		result = localSimulate(protocol, tool, target, capability)
	}

	fmt.Print(formatSimulateOutput(result))
}

type simulateResponse struct {
	Action   string                        `json:"action"`
	Decision string                        `json:"decision"`
	Trace    *toolpolicy.PolicyDecisionTrace `json:"trace"`
	Local    bool                          `json:"-"`
}

func remoteSimulate(adminURL, protocol, tool, target, capability string) (*simulateResponse, error) {
	body := map[string]string{
		"protocol":   protocol,
		"tool":       tool,
		"target":     target,
		"capability": capability,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	resp, err := client.Post(adminURL+"/admin/v1/simulate", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result simulateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func localSimulate(protocol, tool, target, capability string) *simulateResponse {
	rules := defaultPolicyRules()
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
	env := envelope.NewEnvelope(actor, "simulate", envelope.Protocol(protocol), tool, target, cap)

	simResult := engine.Simulate(env)
	return &simulateResponse{
		Action:   simResult.Action,
		Decision: simResult.Decision,
		Trace:    simResult.Trace,
		Local:    true,
	}
}

func formatSimulateOutput(r *simulateResponse) string {
	var sb strings.Builder

	if r.Local {
		sb.WriteString("(local evaluation - admin server not reachable)\n\n")
	}

	// Decision
	switch r.Decision {
	case "allow":
		sb.WriteString(fmt.Sprintf("Decision:       %sALLOWED%s\n", colorGreen, colorReset))
	case "review":
		sb.WriteString(fmt.Sprintf("Decision:       %sREVIEW REQUIRED%s\n", colorYellow, colorReset))
	case "block":
		sb.WriteString(fmt.Sprintf("Decision:       %sBLOCKED%s\n", colorRed, colorReset))
	default:
		sb.WriteString(fmt.Sprintf("Decision:       %s\n", r.Decision))
	}

	sb.WriteString(fmt.Sprintf("Action:         %s\n", r.Action))

	if r.Trace != nil {
		if r.Trace.DefaultUsed {
			sb.WriteString("Matched rule:   (default)\n")
		} else if r.Trace.MatchedRule != nil {
			sb.WriteString(fmt.Sprintf("Matched rule:   #%d [protocol=%s tool=%s target=%s cap=%s -> %s]\n",
				r.Trace.MatchedIndex,
				r.Trace.MatchedRule.Protocol,
				r.Trace.MatchedRule.Tool,
				r.Trace.MatchedRule.Target,
				r.Trace.MatchedRule.Capability,
				r.Trace.MatchedRule.Decision,
			))
		}
		sb.WriteString(fmt.Sprintf("Rules checked:  %d\n", r.Trace.RulesChecked))

		sb.WriteString("\nTrace:\n")
		for _, step := range r.Trace.CheckTrace {
			status := "MISS"
			if step.Matched {
				status = "HIT "
			}
			reason := ""
			if step.FailReason != "" {
				reason = " (" + step.FailReason + ")"
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s  protocol=%s tool=%s target=%s cap=%s -> %s%s\n",
				step.RuleIndex, status,
				step.Rule.Protocol, step.Rule.Tool, step.Rule.Target,
				step.Rule.Capability, step.Rule.Decision, reason,
			))
		}
	}

	return sb.String()
}

// --- aegisctl why ---

func cmdWhy(adminURL string, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: aegisctl why <envelope-id>")
		os.Exit(1)
	}
	id := args[0]

	resp, err := client.Get(adminURL + "/admin/v1/actions/" + id + "/why")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error (%d): could not read response body\n", resp.StatusCode)
		} else {
			fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, string(body))
		}
		os.Exit(1)
	}

	var result simulateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(formatSimulateOutput(&result))
}

// --- aegisctl diff-policy ---

type policyFile struct {
	DefaultDecision string              `yaml:"default_decision"`
	Rules           []toolpolicy.ToolRule `yaml:"rules"`
}

func cmdDiffPolicy(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: aegisctl diff-policy <old.yaml> <new.yaml>")
		os.Exit(1)
	}

	oldPolicy, err := loadPolicyFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", args[0], err)
		os.Exit(1)
	}
	newPolicy, err := loadPolicyFile(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", args[1], err)
		os.Exit(1)
	}

	// Build synthetic test actions from all rules in both files to check impact.
	testActions := buildTestActions(oldPolicy.Rules, newPolicy.Rules)

	diff := toolpolicy.DiffPolicies(oldPolicy.Rules, newPolicy.Rules, testActions)
	fmt.Print(formatDiffOutput(diff))
}

func loadPolicyFile(path string) (*policyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pf policyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, err
	}
	return &pf, nil
}

// buildTestActions creates synthetic envelopes from all rules in both sets so
// the diff can detect impact on representative actions.
func buildTestActions(oldRules, newRules []toolpolicy.ToolRule) []*envelope.ActionEnvelope {
	seen := make(map[string]bool)
	var actions []*envelope.ActionEnvelope

	add := func(rules []toolpolicy.ToolRule) {
		for _, r := range rules {
			proto := r.Protocol
			if proto == "" || proto == "*" {
				proto = "mcp"
			}
			tool := r.Tool
			if tool == "" || tool == "*" {
				tool = "test.tool"
			}
			target := r.Target
			if target == "" || target == "*" {
				target = "test-target"
			}
			cap := r.Capability
			if cap == "" || cap == "*" {
				cap = "execute"
			}
			key := proto + "/" + tool + "/" + target + "/" + cap
			if seen[key] {
				continue
			}
			seen[key] = true
			actions = append(actions, &envelope.ActionEnvelope{
				ID:                  "diff-test",
				Protocol:            envelope.Protocol(proto),
				Tool:                tool,
				Target:              target,
				RequestedCapability: envelope.Capability(cap),
				Actor:               envelope.ActorInfo{Type: "agent", ID: "diff-test", TenantID: "test"},
				Task:                "diff-test",
			})
		}
	}

	add(oldRules)
	add(newRules)
	return actions
}

func formatDiffOutput(diff *toolpolicy.DiffResult) string {
	var sb strings.Builder

	sb.WriteString("Policy Diff\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	if len(diff.Added) > 0 {
		sb.WriteString(fmt.Sprintf("\n%sAdded rules:%s\n", colorGreen, colorReset))
		for _, r := range diff.Added {
			sb.WriteString(fmt.Sprintf("  + [protocol=%s tool=%s target=%s cap=%s -> %s]\n",
				r.Protocol, r.Tool, r.Target, r.Capability, r.Decision))
		}
	}

	if len(diff.Removed) > 0 {
		sb.WriteString(fmt.Sprintf("\n%sRemoved rules:%s\n", colorRed, colorReset))
		for _, r := range diff.Removed {
			sb.WriteString(fmt.Sprintf("  - [protocol=%s tool=%s target=%s cap=%s -> %s]\n",
				r.Protocol, r.Tool, r.Target, r.Capability, r.Decision))
		}
	}

	if len(diff.Changed) > 0 {
		sb.WriteString(fmt.Sprintf("\n%sChanged rules:%s\n", colorYellow, colorReset))
		for _, c := range diff.Changed {
			sb.WriteString(fmt.Sprintf("  ~ [%d] %s -> %s\n", c.Index, c.Before.Decision, c.After.Decision))
			sb.WriteString(fmt.Sprintf("    before: protocol=%s tool=%s target=%s cap=%s\n",
				c.Before.Protocol, c.Before.Tool, c.Before.Target, c.Before.Capability))
			sb.WriteString(fmt.Sprintf("    after:  protocol=%s tool=%s target=%s cap=%s\n",
				c.After.Protocol, c.After.Tool, c.After.Target, c.After.Capability))
		}
	}

	if len(diff.Impact) > 0 {
		sb.WriteString(fmt.Sprintf("\n%sImpacted actions:%s\n", colorRed, colorReset))
		for _, ia := range diff.Impact {
			sb.WriteString(fmt.Sprintf("  ! %s (%s): %s -> %s\n",
				ia.Tool, ia.Protocol, ia.OldDecision, ia.NewDecision))
		}
	}

	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Changed) == 0 {
		sb.WriteString("\nNo differences found.\n")
	}

	return sb.String()
}

// defaultPolicyRules returns the same default rules used by test-action for
// local evaluation.
func defaultPolicyRules() []toolpolicy.ToolRule {
	return []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "rm", Decision: "block"},
		{Protocol: "shell", Tool: "shutdown", Decision: "block"},
		{Protocol: "shell", Tool: "reboot", Decision: "block"},
		{Protocol: "shell", Tool: "mkfs", Decision: "block"},
		{Protocol: "sql", Tool: "*", Capability: "delete", Decision: "review"},
		{Protocol: "sql", Tool: "drop_*", Decision: "block"},
		{Protocol: "git", Tool: "delete_*", Decision: "block"},
		{Protocol: "git", Tool: "merge_*", Capability: "deploy", Decision: "review"},
		{Protocol: "git", Tool: "push", Decision: "review"},
		{Protocol: "*", Tool: "list_*", Capability: "read", Decision: "allow"},
		{Protocol: "*", Tool: "get_*", Capability: "read", Decision: "allow"},
	}
}
