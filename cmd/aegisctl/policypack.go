package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

const defaultPolicyPackDir = "configs/policy-packs"

// PolicyPack represents a policy pack YAML file.
type PolicyPack struct {
	Name            string           `yaml:"name" json:"name"`
	Description     string           `yaml:"description" json:"description"`
	DefaultDecision string           `yaml:"default_decision" json:"default_decision"`
	Rules           []PolicyPackRule `yaml:"rules" json:"rules"`
}

// PolicyPackRule is a single rule in a policy pack.
type PolicyPackRule struct {
	Protocol   string `yaml:"protocol" json:"protocol"`
	Tool       string `yaml:"tool" json:"tool"`
	Target     string `yaml:"target,omitempty" json:"target,omitempty"`
	Capability string `yaml:"capability,omitempty" json:"capability,omitempty"`
	Decision   string `yaml:"decision" json:"decision"`
}

// LoadPolicyPack reads and parses a policy pack YAML file.
func LoadPolicyPack(path string) (*PolicyPack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy pack: %w", err)
	}
	var pack PolicyPack
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("parsing policy pack: %w", err)
	}
	if pack.Name == "" {
		return nil, fmt.Errorf("policy pack missing 'name' field")
	}
	if pack.DefaultDecision == "" {
		return nil, fmt.Errorf("policy pack %q missing 'default_decision' field", pack.Name)
	}
	if len(pack.Rules) == 0 {
		return nil, fmt.Errorf("policy pack %q has no rules", pack.Name)
	}
	for i, r := range pack.Rules {
		if r.Decision == "" {
			return nil, fmt.Errorf("policy pack %q: rule %d missing 'decision'", pack.Name, i)
		}
		switch r.Decision {
		case "allow", "review", "block":
			// valid
		default:
			return nil, fmt.Errorf("policy pack %q: rule %d invalid decision %q", pack.Name, i, r.Decision)
		}
	}
	return &pack, nil
}

// ListPolicyPacks finds all .yaml files in the packs directory.
func ListPolicyPacks(dir string) ([]PolicyPack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading policy packs directory: %w", err)
	}

	var packs []PolicyPack
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		pack, err := LoadPolicyPack(filepath.Join(dir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", e.Name(), err)
			continue
		}
		packs = append(packs, *pack)
	}
	return packs, nil
}

func cmdPolicyPackList(args []string) {
	dir := defaultPolicyPackDir
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		}
	}

	packs, err := ListPolicyPacks(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(packs) == 0 {
		fmt.Println("No policy packs found.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDEFAULT\tRULES\tDESCRIPTION")
	fmt.Fprintln(tw, "────\t───────\t─────\t───────────")
	for _, p := range packs {
		desc := p.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", p.Name, p.DefaultDecision, len(p.Rules), desc)
	}
	tw.Flush()
}

func cmdPolicyPackShow(name string, args []string) {
	dir := defaultPolicyPackDir
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		}
	}

	packs, err := ListPolicyPacks(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var found *PolicyPack
	for _, p := range packs {
		if p.Name == name {
			found = &p
			break
		}
	}

	if found == nil {
		fmt.Fprintf(os.Stderr, "Policy pack %q not found\n", name)
		os.Exit(1)
	}

	fmt.Printf("Name:            %s\n", found.Name)
	fmt.Printf("Description:     %s\n", found.Description)
	fmt.Printf("Default:         %s\n", found.DefaultDecision)
	fmt.Printf("Rules:           %d\n", len(found.Rules))
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROTOCOL\tTOOL\tTARGET\tCAPABILITY\tDECISION")
	fmt.Fprintln(tw, "────────\t────\t──────\t──────────\t────────")
	for _, r := range found.Rules {
		target := r.Target
		if target == "" {
			target = "*"
		}
		cap := r.Capability
		if cap == "" {
			cap = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Protocol, r.Tool, target, cap, strings.ToUpper(r.Decision))
	}
	tw.Flush()
}
