package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func cmdVerify(adminURL string, args []string) {
	sessionID := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--session" && i+1 < len(args) {
			sessionID = args[i+1]
			i++
		}
	}

	var url string
	if sessionID != "" {
		url = adminURL + "/admin/v1/evidence/sessions/" + sessionID + "/verify"
	} else {
		url = adminURL + "/admin/v1/audit/verify"
	}

	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result VerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	printVerifyResult(result)
}

// VerifyResponse matches the evidence.VerifyResult JSON structure.
type VerifyResponse struct {
	Valid        bool   `json:"valid"`
	TotalRecords int    `json:"total_records"`
	ErrorAtIndex int    `json:"error_at_index,omitempty"`
	Message      string `json:"message"`
}

func printVerifyResult(r VerifyResponse) {
	if r.Valid {
		fmt.Printf("\033[32mPASS\033[0m  Evidence chain integrity verified\n")
	} else {
		fmt.Printf("\033[31mFAIL\033[0m  Evidence chain verification failed\n")
	}
	fmt.Printf("  Total entries: %d\n", r.TotalRecords)
	if !r.Valid && r.ErrorAtIndex > 0 {
		fmt.Printf("  Error at index: %d\n", r.ErrorAtIndex)
	}
	if r.Message != "" {
		fmt.Printf("  Message: %s\n", r.Message)
	}
}

// FormatVerifyResult returns the verify output as a string (for testing).
func formatVerifyResult(r VerifyResponse) string {
	var sb strings.Builder
	if r.Valid {
		sb.WriteString("PASS  Evidence chain integrity verified\n")
	} else {
		sb.WriteString("FAIL  Evidence chain verification failed\n")
	}
	sb.WriteString(fmt.Sprintf("  Total entries: %d\n", r.TotalRecords))
	if !r.Valid && r.ErrorAtIndex > 0 {
		sb.WriteString(fmt.Sprintf("  Error at index: %d\n", r.ErrorAtIndex))
	}
	if r.Message != "" {
		sb.WriteString(fmt.Sprintf("  Message: %s\n", r.Message))
	}
	return sb.String()
}
