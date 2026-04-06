//go:build ignore

// Standalone governance overhead benchmark for AegisFlow.
// Measures the latency cost of the runtime governance pipeline so enterprises
// can see the exact overhead of policy evaluation, evidence recording, and
// credential issuance.
//
// Usage:
//
//	go run ./scripts/benchmark_governance.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/credential"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

const iterations = 10_000

// scenarioResult holds the benchmark results for a single scenario.
type scenarioResult struct {
	Name   string  `json:"name"`
	P50    float64 `json:"p50_us"`
	P95    float64 `json:"p95_us"`
	P99    float64 `json:"p99_us"`
	OpsSec float64 `json:"ops_sec"`
}

func main() {
	engine := buildEngine()
	chain := evidence.NewSessionChain("bench-session")
	registry := buildRegistry()
	queue := approval.NewQueue(iterations + 1000)

	scenarios := []struct {
		name string
		fn   func()
	}{
		{
			name: "Policy evaluation only (20 rules)",
			fn: func() {
				env := makeEnvelope()
				engine.Evaluate(env)
			},
		},
		{
			name: "Policy + evidence recording",
			fn: func() {
				env := makeEnvelope()
				d := engine.Evaluate(env)
				env.PolicyDecision = d
				chain.Record(env)
			},
		},
		{
			name: "Full allow (policy + evidence + credential)",
			fn: func() {
				env := makeEnvelope()
				d := engine.Evaluate(env)
				env.PolicyDecision = d
				chain.Record(env)
				registry.Issue(context.Background(), "static", credential.CredentialRequest{
					TaskID:     "bench-task",
					SessionID:  "bench-session",
					TenantID:   "bench-tenant",
					Tool:       env.Tool,
					Target:     env.Target,
					Capability: string(env.RequestedCapability),
					TTL:        5 * time.Minute,
				}, env.ID)
			},
		},
		{
			name: "Review path (policy + queue submit)",
			fn: func() {
				env := makeReviewEnvelope()
				d := engine.Evaluate(env)
				env.PolicyDecision = d
				queue.Submit(env)
			},
		},
	}

	results := make([]scenarioResult, 0, len(scenarios))
	for _, s := range scenarios {
		r := runScenario(s.name, s.fn)
		results = append(results, r)
	}

	printTable(results)
	printJSON(results)
}

func runScenario(name string, fn func()) scenarioResult {
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		fn()
		durations[i] = time.Since(start)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50 := percentile(durations, 50)
	p95 := percentile(durations, 95)
	p99 := percentile(durations, 99)

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	opsSec := float64(iterations) / total.Seconds()

	return scenarioResult{
		Name:   name,
		P50:    float64(p50.Nanoseconds()) / 1000.0,
		P95:    float64(p95.Nanoseconds()) / 1000.0,
		P99:    float64(p99.Nanoseconds()) / 1000.0,
		OpsSec: math.Round(opsSec),
	}
}

func percentile(sorted []time.Duration, pct int) time.Duration {
	idx := int(math.Ceil(float64(pct)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func buildEngine() *toolpolicy.Engine {
	rules := make([]toolpolicy.ToolRule, 0, 20)
	// 18 non-matching rules to simulate realistic evaluation depth.
	protocols := []string{"mcp", "http", "shell", "sql", "git"}
	for i := 0; i < 18; i++ {
		rules = append(rules, toolpolicy.ToolRule{
			Protocol:   protocols[i%len(protocols)],
			Tool:       fmt.Sprintf("decoy.tool_%d", i),
			Decision:   "block",
			Capability: "delete",
		})
	}
	// Rule 19: matches review envelopes.
	rules = append(rules, toolpolicy.ToolRule{
		Protocol: "shell",
		Tool:     "shell.exec",
		Target:   "terraform*",
		Decision: "review",
	})
	// Rule 20: matches benchmark allow envelopes.
	rules = append(rules, toolpolicy.ToolRule{
		Protocol: "mcp",
		Tool:     "github.list_*",
		Decision: "allow",
	})
	return toolpolicy.NewEngine(rules, "block")
}

func buildRegistry() *credential.Registry {
	reg := credential.NewRegistry()
	broker := credential.NewStaticBroker("static", "bench-token-value", 5*time.Minute)
	reg.Register("static", broker)
	return reg
}

func makeEnvelope() *envelope.ActionEnvelope {
	return envelope.NewEnvelope(
		envelope.ActorInfo{
			Type:      "agent",
			ID:        "bench-agent",
			SessionID: "bench-session",
			TenantID:  "bench-tenant",
		},
		"benchmark-task",
		envelope.ProtocolMCP,
		"github.list_repos",
		"github.com/org",
		envelope.CapRead,
	)
}

func makeReviewEnvelope() *envelope.ActionEnvelope {
	return envelope.NewEnvelope(
		envelope.ActorInfo{
			Type:      "agent",
			ID:        "bench-agent",
			SessionID: "bench-session",
			TenantID:  "bench-tenant",
		},
		"benchmark-task",
		envelope.ProtocolShell,
		"shell.exec",
		"terraform apply",
		envelope.CapDeploy,
	)
}

func printTable(results []scenarioResult) {
	fmt.Println("AegisFlow Governance Overhead Benchmarks")
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("%-45s %10s %10s %10s %12s\n", "Scenario", "p50 (μs)", "p95 (μs)", "p99 (μs)", "Ops/sec")
	fmt.Println(strings.Repeat("-", 90))
	for _, r := range results {
		fmt.Printf("%-45s %10.2f %10.2f %10.2f %12.0f\n",
			r.Name, r.P50, r.P95, r.P99, r.OpsSec)
	}
	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("Iterations per scenario: %d\n\n", iterations)
}

func printJSON(results []scenarioResult) {
	output := struct {
		Benchmark  string           `json:"benchmark"`
		Iterations int              `json:"iterations"`
		Results    []scenarioResult `json:"results"`
	}{
		Benchmark:  "AegisFlow Governance Overhead",
		Iterations: iterations,
		Results:    results,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
