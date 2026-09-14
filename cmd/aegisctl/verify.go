package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

func cmdVerify(adminURL string, args []string) {
	sessionID := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--session" || sessionID != "" || i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
			_, err := fmt.Fprintln(os.Stderr, "Usage: aegisctl verify [--session <id>]")
			if err != nil {
				return
			}
			os.Exit(1)
		}
		sessionID = args[i+1]
		i++
	}

	var url string
	if sessionID != "" {
		url = adminURL + "/admin/v1/evidence/sessions/" + sessionID + "/verify"
	} else {
		url = adminURL + "/admin/v1/audit/verify"
	}

	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	defer cleanup.Close(resp.Body)

	var result VerifyResponse
	if err := decodeJSON(resp, &result); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	if key := os.Getenv("AEGISFLOW_API_KEY"); key != "" {
		result.Message = strings.ReplaceAll(result.Message, key, "[redacted]")
	}

	printVerifyResult(result)
	if !result.Valid {
		os.Exit(1)
	}
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
	_, err := fmt.Fprintf(&sb, "  Total entries: %d\n", r.TotalRecords)
	if err != nil {
		return ""
	}
	if !r.Valid && r.ErrorAtIndex > 0 {
		_, err := fmt.Fprintf(&sb, "  Error at index: %d\n", r.ErrorAtIndex)
		if err != nil {
			return ""
		}
	}
	if r.Message != "" {
		_, err := fmt.Fprintf(&sb, "  Message: %s\n", r.Message)
		if err != nil {
			return ""
		}
	}
	return sb.String()
}
