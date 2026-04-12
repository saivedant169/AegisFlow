package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func cmdEvidenceSessions(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/evidence/sessions")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var sessions []SessionSummary
	if err := json.Unmarshal(body, &sessions); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No evidence sessions found.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION ID\tACTIONS\tVALID\tSTARTED\tLAST HASH")
	fmt.Fprintln(tw, "──────────\t───────\t─────\t───────\t─────────")
	for _, s := range sessions {
		valid := "yes"
		if !s.ChainValid {
			valid = "no"
		}
		lastHash := s.LastHash
		if len(lastHash) > 12 {
			lastHash = lastHash[:12] + "..."
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
			s.SessionID, s.TotalActions, valid, s.StartedAt, lastHash)
	}
	tw.Flush()
}

func cmdEvidenceExport(adminURL string, sessionID string, args []string) {
	outputFile := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--file" && i+1 < len(args) {
			outputFile = args[i+1]
			i++
		}
	}

	resp, err := client.Get(adminURL + "/admin/v1/evidence/sessions/" + sessionID + "/export")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Pretty-print the JSON
	var pretty json.RawMessage
	if err := json.Unmarshal(body, &pretty); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	formatted, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, formatted, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Exported session %s to %s\n", sessionID, outputFile)
	} else {
		fmt.Println(string(formatted))
	}
}

// SessionSummary matches the evidence.SessionManifest JSON structure.
type SessionSummary struct {
	SessionID    string `json:"session_id"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	TotalActions int    `json:"total_actions"`
	Allowed      int    `json:"allowed"`
	Reviewed     int    `json:"reviewed"`
	Blocked      int    `json:"blocked"`
	FirstHash    string `json:"first_hash"`
	LastHash     string `json:"last_hash"`
	ChainValid   bool   `json:"chain_valid"`
}
