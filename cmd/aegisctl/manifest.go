package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

func cmdManifestCreate(adminURL string, args []string) {
	var taskID, tools, protocols, verbs, owner, description, riskTier, expiresIn string
	var maxActions int

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 < len(args) {
				taskID = args[i+1]
				i++
			}
		case "--tools":
			if i+1 < len(args) {
				tools = args[i+1]
				i++
			}
		case "--protocols":
			if i+1 < len(args) {
				protocols = args[i+1]
				i++
			}
		case "--verbs":
			if i+1 < len(args) {
				verbs = args[i+1]
				i++
			}
		case "--owner":
			if i+1 < len(args) {
				owner = args[i+1]
				i++
			}
		case "--description":
			if i+1 < len(args) {
				description = args[i+1]
				i++
			}
		case "--risk-tier":
			if i+1 < len(args) {
				riskTier = args[i+1]
				i++
			}
		case "--max-actions":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxActions)
				i++
			}
		case "--expires-in":
			if i+1 < len(args) {
				expiresIn = args[i+1]
				i++
			}
		}
	}

	if taskID == "" {
		fmt.Println("Usage: aegisctl manifest create --task <TICKET-ID> [--tools \"glob,...\"] [--protocols \"proto,...\"] [--verbs \"verb,...\"] [--max-actions N] [--owner NAME] [--description DESC] [--risk-tier low|medium|high] [--expires-in 1h]")
		os.Exit(1)
	}

	body := map[string]interface{}{
		"task_id":     taskID,
		"description": description,
		"owner":       owner,
		"risk_tier":   riskTier,
		"max_actions": maxActions,
	}
	if expiresIn != "" {
		body["expires_in"] = expiresIn
	}
	if tools != "" {
		body["allowed_tools"] = strings.Split(tools, ",")
	}
	if protocols != "" {
		body["allowed_protocols"] = strings.Split(protocols, ",")
	}
	if verbs != "" {
		body["allowed_verbs"] = strings.Split(verbs, ",")
	}

	data, err := marshalJSON(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	resp, err := client.Post(adminURL+"/admin/v1/manifests", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respData, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, string(respData))
		os.Exit(1)
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Manifest created:\n")
	fmt.Printf("  ID:        %v\n", result["id"])
	fmt.Printf("  Task:      %v\n", result["task_id"])
	fmt.Printf("  Hash:      %v\n", result["manifest_hash"])
	fmt.Printf("  Risk Tier: %v\n", result["risk_tier"])
	fmt.Printf("  Active:    %v\n", result["active"])
}

func cmdManifestList(adminURL string) {
	resp, err := client.Get(adminURL + "/admin/v1/manifests")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var manifests []map[string]interface{}
	if err := decodeJSON(resp, &manifests); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if len(manifests) == 0 {
		fmt.Println("No active manifests.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTASK\tOWNER\tRISK\tACTIVE\tHASH")
	fmt.Fprintln(tw, "──\t────\t─────\t────\t──────\t────")
	for _, m := range manifests {
		id := fmt.Sprintf("%v", m["id"])
		if len(id) > 20 {
			id = id[:17] + "..."
		}
		hash := fmt.Sprintf("%v", m["manifest_hash"])
		if len(hash) > 12 {
			hash = hash[:12] + "..."
		}
		fmt.Fprintf(tw, "%s\t%v\t%v\t%v\t%v\t%s\n",
			id, m["task_id"], m["owner"], m["risk_tier"], m["active"], hash)
	}
	tw.Flush()
}

func cmdManifestDrift(adminURL string, id string) {
	resp, err := client.Get(adminURL + "/admin/v1/manifests/" + id + "/drift")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var events []map[string]interface{}
	if err := decodeJSON(resp, &events); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if len(events) == 0 {
		fmt.Println("No drift events recorded.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tTOOL\tPROTOCOL\tSEVERITY\tMESSAGE")
	fmt.Fprintln(tw, "────\t────\t────────\t────────\t───────")
	for _, e := range events {
		msg := fmt.Sprintf("%v", e["message"])
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\t%s\n",
			e["type"], e["tool"], e["protocol"], e["severity"], msg)
	}
	tw.Flush()
}
