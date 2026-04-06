package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/saivedant169/AegisFlow/internal/supply"
)

func cmdSupplyChainList(adminURL string) {
	data := fetchJSON(adminURL + "/admin/v1/supply-chain")
	if data == nil {
		return
	}

	raw, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	var resp struct {
		Assets []supply.LoadedAsset `json:"assets"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return
	}

	if len(resp.Assets) == 0 {
		fmt.Println("No loaded supply chain assets.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVERSION\tTYPE\tTRUST TIER\tVERIFIED\tLOADED AT")
	fmt.Fprintln(tw, "────\t───────\t────\t──────────\t────────\t─────────")
	for _, a := range resp.Assets {
		verified := "no"
		if a.Verified {
			verified = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			a.Name, a.Version, a.Type, a.TrustTier, verified,
			a.LoadedAt.Format("2006-01-02 15:04:05"))
	}
	tw.Flush()
}

func cmdSupplyChainSign(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: aegisctl supply-chain sign <file> --key <hex-key> [--name NAME] [--version VERSION] [--type TYPE]")
		os.Exit(1)
	}

	filePath := args[0]
	var keyHex, name, version, bundleType string
	version = "0.0.0"
	bundleType = "policy_pack"

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--key":
			if i+1 < len(args) {
				keyHex = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--version":
			if i+1 < len(args) {
				version = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				bundleType = args[i+1]
				i++
			}
		}
	}

	if keyHex == "" {
		fmt.Fprintln(os.Stderr, "Error: --key is required")
		os.Exit(1)
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid hex key: %v\n", err)
		os.Exit(1)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	if name == "" {
		name = filePath
	}

	signer := supply.NewSigner(key)
	bundle, err := signer.Sign(name, version, bundleType, content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error signing: %v\n", err)
		os.Exit(1)
	}

	sigFile := filePath + ".sig"
	sigData, _ := json.MarshalIndent(bundle, "", "  ")
	if err := os.WriteFile(sigFile, sigData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing signature: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Signed: %s\n", filePath)
	fmt.Printf("Signature written to: %s\n", sigFile)
	fmt.Printf("Content hash: %s\n", bundle.ContentHash)
	fmt.Printf("Trust tier: %s\n", bundle.TrustTier)
}

func cmdSupplyChainVerify(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: aegisctl supply-chain verify <file> --signature <sig-file> --key <hex-key>")
		os.Exit(1)
	}

	filePath := args[0]
	var sigFile, keyHex string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--signature":
			if i+1 < len(args) {
				sigFile = args[i+1]
				i++
			}
		case "--key":
			if i+1 < len(args) {
				keyHex = args[i+1]
				i++
			}
		}
	}

	if sigFile == "" {
		sigFile = filePath + ".sig"
	}
	if keyHex == "" {
		fmt.Fprintln(os.Stderr, "Error: --key is required")
		os.Exit(1)
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid hex key: %v\n", err)
		os.Exit(1)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	sigData, err := os.ReadFile(sigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading signature file: %v\n", err)
		os.Exit(1)
	}

	var bundle supply.SignedBundle
	if err := json.Unmarshal(sigData, &bundle); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing signature file: %v\n", err)
		os.Exit(1)
	}

	signer := supply.NewSigner(key)
	if err := signer.Verify(&bundle, content); err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VERIFIED: %s\n", bundle.Name)
	fmt.Printf("Version:      %s\n", bundle.Version)
	fmt.Printf("Type:         %s\n", bundle.Type)
	fmt.Printf("Trust tier:   %s\n", bundle.TrustTier)
	fmt.Printf("Signed by:    %s\n", bundle.SignedBy)
	fmt.Printf("Signed at:    %s\n", bundle.SignedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Content hash: %s\n", bundle.ContentHash)
}
