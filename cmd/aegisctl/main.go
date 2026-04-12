package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	defaultGatewayURL = "http://localhost:8080"
	defaultAdminURL   = "http://localhost:8081"
)

var client = &http.Client{Timeout: 10 * time.Second}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	adminURL := getEnv("AEGISFLOW_ADMIN_URL", defaultAdminURL)
	gatewayURL := getEnv("AEGISFLOW_GATEWAY_URL", defaultGatewayURL)

	switch os.Args[1] {
	case "plugin":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl plugin <search|info|install|list|outdated|remove> [args]")
			os.Exit(1)
		}
		var err error
		switch os.Args[2] {
		case "search":
			err = pluginSearch(os.Args[3:])
		case "info":
			err = pluginInfo(os.Args[3:])
		case "install":
			err = pluginInstall(os.Args[3:])
		case "list":
			err = pluginList(os.Args[3:])
		case "outdated":
			err = pluginOutdated(os.Args[3:])
		case "remove":
			err = pluginRemove(os.Args[3:])
		default:
			fmt.Printf("Unknown plugin command: %s\n", os.Args[2])
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		cmdStatus(gatewayURL, adminURL)
	case "usage":
		cmdUsage(adminURL)
	case "models":
		cmdModels(gatewayURL)
	case "providers":
		cmdProviders(adminURL)
	case "policies":
		cmdPolicies(adminURL)
	case "tenants":
		cmdTenants(adminURL)
	case "pending":
		cmdPending(adminURL)
	case "approve":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl approve <id> [comment]")
			os.Exit(1)
		}
		comment := ""
		if len(os.Args) > 3 {
			comment = strings.Join(os.Args[3:], " ")
		}
		cmdApprove(adminURL, os.Args[2], comment)
	case "deny":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl deny <id> [comment]")
			os.Exit(1)
		}
		comment := ""
		if len(os.Args) > 3 {
			comment = strings.Join(os.Args[3:], " ")
		}
		cmdDeny(adminURL, os.Args[2], comment)
	case "verify":
		cmdVerify(adminURL, os.Args[2:])
	case "evidence":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl evidence <sessions|export|report> [args]")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "sessions":
			cmdEvidenceSessions(adminURL)
		case "export":
			if len(os.Args) < 4 {
				fmt.Println("Usage: aegisctl evidence export <session-id> [--file output.json]")
				os.Exit(1)
			}
			cmdEvidenceExport(adminURL, os.Args[3], os.Args[4:])
		case "report":
			if len(os.Args) < 4 {
				fmt.Println("Usage: aegisctl evidence report <session-id> [--html] [--file output.md]")
				os.Exit(1)
			}
			cmdEvidenceReport(adminURL, os.Args[3], os.Args[4:])
		default:
			fmt.Printf("Unknown evidence command: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "policy-pack":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl policy-pack <list|show> [args]")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			cmdPolicyPackList(os.Args[3:])
		case "show":
			if len(os.Args) < 4 {
				fmt.Println("Usage: aegisctl policy-pack show <name> [--dir DIR]")
				os.Exit(1)
			}
			cmdPolicyPackShow(os.Args[3], os.Args[4:])
		default:
			fmt.Printf("Unknown policy-pack command: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "simulate":
		cmdSimulate(adminURL, os.Args[2:])
	case "why":
		cmdWhy(adminURL, os.Args[2:])
	case "diff-policy":
		cmdDiffPolicy(os.Args[2:])
	case "manifest":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl manifest <create|list|drift> [args]")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "create":
			cmdManifestCreate(adminURL, os.Args[3:])
		case "list":
			cmdManifestList(adminURL)
		case "drift":
			if len(os.Args) < 4 {
				fmt.Println("Usage: aegisctl manifest drift <manifest-id>")
				os.Exit(1)
			}
			cmdManifestDrift(adminURL, os.Args[3])
		default:
			fmt.Printf("Unknown manifest command: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "supply-chain":
		if len(os.Args) < 3 {
			fmt.Println("Usage: aegisctl supply-chain <list|sign|verify> [args]")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			cmdSupplyChainList(adminURL)
		case "sign":
			cmdSupplyChainSign(os.Args[3:])
		case "verify":
			cmdSupplyChainVerify(os.Args[3:])
		default:
			fmt.Printf("Unknown supply-chain command: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "test-action":
		cmdTestAction(adminURL, os.Args[2:])
	case "test":
		apiKey := "aegis-test-default-001"
		model := "mock"
		msg := "Hello from aegisctl!"
		if len(os.Args) > 2 {
			msg = strings.Join(os.Args[2:], " ")
		}
		cmdTest(gatewayURL, apiKey, model, msg)
	case "help", "--help", "-h":
		printUsage()
	case "version":
		fmt.Println("aegisctl v0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`aegisctl — AegisFlow CLI

Usage: aegisctl <command> [args]

Commands:
  verify      Verify evidence chain integrity (--session <id> for specific session)
  evidence    Evidence management (sessions, export, report)
  policy-pack Manage policy packs (list, show)
  plugin      Manage WASM plugins (search, info, install, list, outdated, remove)
  status      Check gateway and admin health
  usage       Show usage per tenant and model
  models      List available models
  providers   List configured providers with health
  policies    List configured policies
  tenants     List tenants with rate limits
  pending     List pending approval items
  approve     Approve a pending item: aegisctl approve <id> [comment]
  deny        Deny a pending item: aegisctl deny <id> [comment]
  simulate    Simulate a policy decision with full trace
  why         Show decision trace for a past action: aegisctl why <envelope-id>
  diff-policy Diff two policy files: aegisctl diff-policy <old.yaml> <new.yaml>
  manifest    Manage task manifests and drift detection (create, list, drift)
  supply-chain Manage supply chain trust (list, sign, verify)
  test-action Simulate an agent action through governance pipeline
  test [msg]  Send a test chat completion
  version     Show version
  help        Show this help

Environment:
  AEGISFLOW_GATEWAY_URL  Gateway URL (default: http://localhost:8080)
  AEGISFLOW_ADMIN_URL    Admin URL (default: http://localhost:8081)`)
}

func cmdStatus(gatewayURL, adminURL string) {
	fmt.Println("AegisFlow Status")
	fmt.Println("════════════════════════════════════════════════════")

	// Health
	gwOK := checkHealth(gatewayURL + "/health")
	adOK := checkHealth(adminURL + "/health")
	fmt.Printf("  Gateway:  %s  (%s)\n", statusIcon(gwOK), gatewayURL)
	fmt.Printf("  Admin:    %s  (%s)\n", statusIcon(adOK), adminURL)

	if !adOK {
		fmt.Println("\nAdmin API unreachable — cannot fetch detailed status.")
		os.Exit(1)
	}

	// Providers
	fmt.Println("\nProviders")
	fmt.Println("────────────────────────────────────────────────────")
	providers := fetchJSON(adminURL + "/admin/v1/providers")
	if provList, ok := providers.([]interface{}); ok && len(provList) > 0 {
		for _, p := range provList {
			prov, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			health := "unhealthy"
			if h, ok := prov["healthy"].(bool); ok && h {
				health = "healthy"
			}
			enabled := "disabled"
			if e, ok := prov["enabled"].(bool); ok && e {
				enabled = "enabled"
			}
			fmt.Printf("  %-20s %s / %s\n", prov["name"], enabled, health)
		}
	} else {
		fmt.Println("  (none configured)")
	}

	// Pending approvals
	fmt.Println("\nApprovals")
	fmt.Println("────────────────────────────────────────────────────")
	approvals := fetchJSON(adminURL + "/admin/v1/approvals")
	pendingCount := 0
	if appData, ok := approvals.(map[string]interface{}); ok {
		if pending, ok := appData["pending"].([]interface{}); ok {
			pendingCount = len(pending)
		}
	} else if appList, ok := approvals.([]interface{}); ok {
		pendingCount = len(appList)
	}
	if pendingCount > 0 {
		fmt.Printf("  %d pending (run: aegisctl pending)\n", pendingCount)
	} else {
		fmt.Println("  No pending approvals")
	}

	// Evidence sessions
	fmt.Println("\nEvidence")
	fmt.Println("────────────────────────────────────────────────────")
	sessions := fetchJSON(adminURL + "/admin/v1/evidence/sessions")
	if sessList, ok := sessions.([]interface{}); ok && len(sessList) > 0 {
		fmt.Printf("  %d active session(s)\n", len(sessList))
		for _, s := range sessList {
			sess, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			valid := "valid"
			if v, ok := sess["chain_valid"].(bool); ok && !v {
				valid = "INVALID"
			}
			fmt.Printf("    %s  actions=%.0f  chain=%s\n",
				sess["session_id"], toFloat(sess["total_actions"]), valid)
		}
	} else {
		fmt.Println("  No active sessions")
	}

	// Budget summary
	fmt.Println("\nBudgets")
	fmt.Println("────────────────────────────────────────────────────")
	budgets := fetchJSON(adminURL + "/admin/v1/budgets")
	if budgetData, ok := budgets.(map[string]interface{}); ok {
		if statuses, ok := budgetData["statuses"].([]interface{}); ok && len(statuses) > 0 {
			for _, st := range statuses {
				s, ok := st.(map[string]interface{})
				if !ok {
					continue
				}
				fmt.Printf("  %-20s spent=$%.2f  limit=$%.2f\n",
					s["tenant_id"], toFloat(s["spent"]), toFloat(s["limit"]))
			}
		} else {
			fmt.Println("  No budget data")
		}
	} else {
		fmt.Println("  Budget tracking not enabled")
	}

	// Recent violations
	fmt.Println("\nRecent Violations")
	fmt.Println("────────────────────────────────────────────────────")
	violations := fetchJSON(adminURL + "/admin/v1/violations")
	if violList, ok := violations.([]interface{}); ok && len(violList) > 0 {
		shown := len(violList)
		if shown > 5 {
			shown = 5
		}
		fmt.Printf("  %d total (showing last %d)\n", len(violList), shown)
		for i := 0; i < shown; i++ {
			v, ok := violList[i].(map[string]interface{})
			if !ok {
				continue
			}
			fmt.Printf("    [%s] %s — %s\n", v["policy_name"], v["tenant_id"], v["action"])
		}
	} else {
		fmt.Println("  No recent violations")
	}

	fmt.Println("\n════════════════════════════════════════════════════")
	if !gwOK {
		fmt.Println("WARNING: Gateway is DOWN")
		os.Exit(1)
	}
	fmt.Println("All systems operational.")
}

func cmdUsage(adminURL string) {
	data := fetchJSON(adminURL + "/admin/v1/usage")
	if data == nil {
		return
	}

	usageMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Println("No usage data")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TENANT\tMODEL\tREQUESTS\tTOKENS\tCOST")
	fmt.Fprintln(w, "──────\t─────\t────────\t──────\t────")

	for tenantID, v := range usageMap {
		tenant, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		byModel, ok := tenant["by_model"].(map[string]interface{})
		if !ok {
			continue
		}
		for model, mv := range byModel {
			m, ok := mv.(map[string]interface{})
			if !ok {
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%.0f\t%.0f\t$%.6f\n",
				tenantID, model,
				toFloat(m["requests"]),
				toFloat(m["total_tokens"]),
				toFloat(m["estimated_cost_usd"]),
			)
		}
	}
	w.Flush()
}

func cmdModels(gatewayURL string) {
	apiKey := getEnv("AEGISFLOW_API_KEY", "aegis-test-default-001")
	req, _ := http.NewRequest("GET", gatewayURL+"/v1/models", nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tPROVIDER")
	fmt.Fprintln(w, "─────\t────────")
	for _, m := range result.Data {
		fmt.Fprintf(w, "%s\t%s\n", m.ID, m.Provider)
	}
	w.Flush()
}

func cmdProviders(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/providers")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var providers []struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Enabled bool     `json:"enabled"`
		Healthy bool     `json:"healthy"`
		Models  []string `json:"models"`
	}
	if err := decodeJSON(resp, &providers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tSTATUS\tHEALTH\tMODELS")
	fmt.Fprintln(w, "────\t────\t──────\t──────\t──────")
	for _, p := range providers {
		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		health := "unhealthy"
		if p.Healthy {
			health = "healthy"
		}
		models := strings.Join(p.Models, ", ")
		if len(models) > 40 {
			models = models[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Type, status, health, models)
	}
	w.Flush()
}

func cmdPolicies(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/policies")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var policies []struct {
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		Phase    string   `json:"phase"`
		Action   string   `json:"action"`
		Keywords []string `json:"keywords"`
		Patterns []string `json:"patterns"`
	}
	if err := decodeJSON(resp, &policies); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tPHASE\tACTION\tRULES")
	fmt.Fprintln(w, "────\t────\t─────\t──────\t─────")
	for _, p := range policies {
		rules := append(p.Keywords, p.Patterns...)
		ruleStr := strings.Join(rules, ", ")
		if len(ruleStr) > 50 {
			ruleStr = ruleStr[:47] + "..."
		}
		if ruleStr == "" {
			ruleStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Type, p.Phase, strings.ToUpper(p.Action), ruleStr)
	}
	w.Flush()
}

func cmdTenants(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/tenants")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var tenants []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		KeyCount          int    `json:"key_count"`
		RequestsPerMinute int    `json:"requests_per_minute"`
		TokensPerMinute   int    `json:"tokens_per_minute"`
	}
	if err := decodeJSON(resp, &tenants); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tKEYS\tREQ/MIN\tTOK/MIN")
	fmt.Fprintln(w, "──\t────\t────\t───────\t───────")
	for _, t := range tenants {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n", t.ID, t.Name, t.KeyCount, t.RequestsPerMinute, t.TokensPerMinute)
	}
	w.Flush()
}

func cmdTest(gatewayURL, apiKey, model, message string) {
	body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"%s"}]}`, model, message)

	req, _ := http.NewRequest("POST", gatewayURL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != 200 {
		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, errMsg)
		os.Exit(1)
	}

	fmt.Printf("Model:    %s\n", model)
	fmt.Printf("Latency:  %s\n", latency.Round(time.Millisecond))
	fmt.Printf("Tokens:   %d\n", result.Usage.TotalTokens)
	fmt.Printf("Response: %s\n", result.Choices[0].Message.Content)
}

func checkHealth(url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func fetchJSON(url string) interface{} {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	var result interface{}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return nil
	}
	return result
}

func statusIcon(ok bool) string {
	if ok {
		return "UP"
	}
	return "DOWN"
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func cmdPending(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/approvals")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	pending, _ := result["pending"].([]interface{})
	if len(pending) == 0 {
		fmt.Println("No pending approvals.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\tTOOL\tPROTOCOL\tACTOR\tSUBMITTED\n")
	for _, p := range pending {
		item, _ := p.(map[string]interface{})
		env, _ := item["envelope"].(map[string]interface{})
		actor, _ := env["actor"].(map[string]interface{})
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			item["id"], env["tool"], env["protocol"], actor["id"], item["submitted_at"])
	}
	tw.Flush()
}

func cmdApprove(adminURL, id, comment string) {
	body, _ := marshalJSON(map[string]string{"reviewer": "aegisctl", "comment": comment})
	resp, err := client.Post(adminURL+"/admin/v1/approvals/"+id+"/approve", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var result map[string]string
		if err := decodeJSON(resp, &result); err != nil {
			fmt.Fprintf(os.Stderr, "Error (%d): could not decode error response: %v\n", resp.StatusCode, err)
		} else {
			fmt.Fprintf(os.Stderr, "Failed: %s\n", result["error"])
		}
		os.Exit(1)
	}
	fmt.Printf("Approved: %s\n", id)
}

func cmdDeny(adminURL, id, comment string) {
	body, _ := marshalJSON(map[string]string{"reviewer": "aegisctl", "comment": comment})
	resp, err := client.Post(adminURL+"/admin/v1/approvals/"+id+"/deny", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var result map[string]string
		if err := decodeJSON(resp, &result); err != nil {
			fmt.Fprintf(os.Stderr, "Error (%d): could not decode error response: %v\n", resp.StatusCode, err)
		} else {
			fmt.Fprintf(os.Stderr, "Failed: %s\n", result["error"])
		}
		os.Exit(1)
	}
	fmt.Printf("Denied: %s\n", id)
}
