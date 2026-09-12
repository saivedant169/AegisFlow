package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

func cmdPolicyHistory(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/policy-versions")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup.Close(resp.Body)

	var versions []struct {
		Version         int    `json:"version"`
		Timestamp       string `json:"timestamp"`
		RuleCount       int    `json:"rule_count"`
		DefaultDecision string `json:"default_decision"`
		Source          string `json:"source"`
	}
	if err := decodeJSON(resp, &versions); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Println("No policy versions recorded.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	checkOutput(fmt.Fprintln(tw, "VERSION\tTIMESTAMP\tRULES\tDEFAULT\tSOURCE"))
	for _, v := range versions {
		ts := v.Timestamp
		if len(ts) > 19 {
			ts = ts[:19]
		}
		checkOutput(fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\n",
			v.Version, ts, v.RuleCount, v.DefaultDecision, v.Source))
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "Error: could not flush output")
		os.Exit(1)
	}
}

func cmdPolicyCurrent(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/policy-versions/current")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup.Close(resp.Body)

	var version map[string]interface{}
	if err := decodeJSON(resp, &version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(version, "", "  ")
	fmt.Println(string(data))
}

func cmdPolicyRollback(adminURL string, versionStr string) {
	url := adminURL + "/admin/v1/policy-versions/" + versionStr + "/rollback"
	req, _ := http.NewRequest("POST", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup.Close(resp.Body)

	if resp.StatusCode != 200 {
		var result map[string]interface{}
		if err := decodeJSON(resp, &result); err != nil {
			fmt.Fprintln(os.Stderr, "Error: invalid rollback response")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error (%d): %v\n", resp.StatusCode, result)
		os.Exit(1)
	}
	fmt.Printf("Rolled back to version %s\n", versionStr)
}
