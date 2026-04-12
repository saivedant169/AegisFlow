package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
)

func cmdPolicyHistory(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/policy-versions")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var versions []struct {
		Version         int    `json:"version"`
		Timestamp       string `json:"timestamp"`
		RuleCount       int    `json:"rule_count"`
		DefaultDecision string `json:"default_decision"`
		Source          string `json:"source"`
	}
	if err := decodeJSON(resp, &versions); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if len(versions) == 0 {
		fmt.Println("No policy versions recorded.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tTIMESTAMP\tRULES\tDEFAULT\tSOURCE")
	for _, v := range versions {
		ts := v.Timestamp
		if len(ts) > 19 {
			ts = ts[:19]
		}
		fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\n",
			v.Version, ts, v.RuleCount, v.DefaultDecision, v.Source)
	}
	tw.Flush()
}

func cmdPolicyCurrent(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/policy-versions/current")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var version map[string]interface{}
	if err := decodeJSON(resp, &version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var result map[string]interface{}
		decodeJSON(resp, &result)
		fmt.Fprintf(os.Stderr, "Error (%d): %v\n", resp.StatusCode, result)
		os.Exit(1)
	}
	fmt.Printf("Rolled back to version %s\n", versionStr)
}
