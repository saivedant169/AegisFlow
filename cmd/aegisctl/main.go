package main

import (
	"encoding/json"
	"fmt"
	"io"
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
			fmt.Println("Usage: aegisctl plugin <search|info|install|list|remove> [args]")
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
  plugin      Manage WASM plugins (search, info, install, list, remove)
  status      Check gateway and admin health
  usage       Show usage per tenant and model
  models      List available models
  providers   List configured providers with health
  policies    List configured policies
  tenants     List tenants with rate limits
  test [msg]  Send a test chat completion
  version     Show version
  help        Show this help

Environment:
  AEGISFLOW_GATEWAY_URL  Gateway URL (default: http://localhost:8080)
  AEGISFLOW_ADMIN_URL    Admin URL (default: http://localhost:8081)`)
}

func cmdStatus(gatewayURL, adminURL string) {
	fmt.Println("AegisFlow Status")
	fmt.Println("─────────────────")

	gwOK := checkHealth(gatewayURL + "/health")
	fmt.Printf("  Gateway  (%s):  %s\n", gatewayURL, statusIcon(gwOK))

	adOK := checkHealth(adminURL + "/health")
	fmt.Printf("  Admin    (%s):  %s\n", adminURL, statusIcon(adOK))

	if !gwOK || !adOK {
		os.Exit(1)
	}
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
	json.NewDecoder(resp.Body).Decode(&result)

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
	json.NewDecoder(resp.Body).Decode(&providers)

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
	json.NewDecoder(resp.Body).Decode(&policies)

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
	json.NewDecoder(resp.Body).Decode(&tenants)

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
	json.NewDecoder(resp.Body).Decode(&result)

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

	data, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(data, &result)
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
