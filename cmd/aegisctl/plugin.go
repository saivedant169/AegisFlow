package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultRegistryURL = "https://raw.githubusercontent.com/aegisflow/plugin-registry/main/registry.json"

var registryURL = defaultRegistryURL

type PluginEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	URL         string   `json:"url"`
	SHA256      string   `json:"sha256"`
	Phase       string   `json:"phase"`
	Action      string   `json:"action"`
	Tags        []string `json:"tags"`
	Updated     string   `json:"updated"`
}

type Registry struct {
	Plugins []PluginEntry `json:"plugins"`
}

type PluginsConfig struct {
	Policies struct {
		Input  []PluginPolicyEntry `yaml:"input"`
		Output []PluginPolicyEntry `yaml:"output"`
	} `yaml:"policies"`
}

type PluginPolicyEntry struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Action  string `yaml:"action"`
	Path    string `yaml:"path"`
	Version string `yaml:"version,omitempty"`
}

type OutdatedPlugin struct {
	Name             string
	InstalledVersion string
	LatestVersion    string
}

func fetchRegistry(url string) (*Registry, error) {
	if url == "" {
		url = registryURL
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}

	var reg Registry
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	return &reg, nil
}

func pluginSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aegisctl plugin search <query>")
	}
	query := strings.ToLower(args[0])

	reg, err := fetchRegistry("")
	if err != nil {
		return err
	}

	found := false
	for _, p := range reg.Plugins {
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Description), query) ||
			containsTag(p.Tags, query) {
			fmt.Printf("%-25s %-10s %s\n", p.Name, p.Version, p.Description)
			found = true
		}
	}
	if !found {
		fmt.Println("No plugins found matching:", query)
	}
	return nil
}

func pluginInfo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aegisctl plugin info <name>")
	}
	reg, err := fetchRegistry("")
	if err != nil {
		return err
	}
	for _, p := range reg.Plugins {
		if p.Name == args[0] {
			fmt.Printf("Name:        %s\n", p.Name)
			fmt.Printf("Version:     %s\n", p.Version)
			fmt.Printf("Author:      %s\n", p.Author)
			fmt.Printf("Description: %s\n", p.Description)
			fmt.Printf("Phase:       %s\n", p.Phase)
			fmt.Printf("Action:      %s\n", p.Action)
			fmt.Printf("Tags:        %s\n", strings.Join(p.Tags, ", "))
			fmt.Printf("URL:         %s\n", p.URL)
			fmt.Printf("SHA256:      %s\n", p.SHA256)
			return nil
		}
	}
	return fmt.Errorf("plugin %q not found", args[0])
}

func pluginInstall(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aegisctl plugin install <name> [--plugins-dir DIR] [--plugins-config FILE]")
	}
	name := args[0]
	pluginsDir := "plugins"
	pluginsConfig := "plugins.yaml"

	// Parse flags
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--plugins-dir":
			if i+1 < len(args) {
				pluginsDir = args[i+1]
				i++
			}
		case "--plugins-config":
			if i+1 < len(args) {
				pluginsConfig = args[i+1]
				i++
			}
		}
	}

	reg, err := fetchRegistry("")
	if err != nil {
		return err
	}

	var plugin *PluginEntry
	for _, p := range reg.Plugins {
		if p.Name == name {
			plugin = &p
			break
		}
	}
	if plugin == nil {
		return fmt.Errorf("plugin %q not found in registry", name)
	}

	// Create plugins directory
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}

	// Validate download URL is HTTPS
	if !strings.HasPrefix(plugin.URL, "https://") {
		return fmt.Errorf("refusing to download from non-HTTPS URL: %s", plugin.URL)
	}

	// Download
	fmt.Printf("Downloading %s v%s...\n", plugin.Name, plugin.Version)
	dlClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := dlClient.Get(plugin.URL)
	if err != nil {
		return fmt.Errorf("downloading plugin: %w", err)
	}
	defer resp.Body.Close()

	const maxPluginSize = 50 * 1024 * 1024 // 50MB max plugin size
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginSize))
	if err != nil {
		return fmt.Errorf("reading plugin data: %w", err)
	}

	// Verify SHA-256
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != plugin.SHA256 {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", plugin.SHA256, hash)
	}
	fmt.Println("SHA-256 verified.")

	// Write file
	wasmPath := filepath.Join(pluginsDir, plugin.Name+".wasm")
	if err := os.WriteFile(wasmPath, data, 0644); err != nil {
		return fmt.Errorf("writing plugin file: %w", err)
	}

	// Update plugins.yaml
	var cfg PluginsConfig
	if existingData, err := os.ReadFile(pluginsConfig); err == nil {
		yaml.Unmarshal(existingData, &cfg)
	}

	entry := PluginPolicyEntry{
		Name:    plugin.Name,
		Type:    "wasm",
		Action:  plugin.Action,
		Path:    wasmPath,
		Version: plugin.Version,
	}

	if plugin.Phase == "output" {
		cfg.Policies.Output = append(cfg.Policies.Output, entry)
	} else {
		cfg.Policies.Input = append(cfg.Policies.Input, entry)
	}

	yamlData, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(pluginsConfig, yamlData, 0644); err != nil {
		return fmt.Errorf("writing plugins config: %w", err)
	}

	fmt.Printf("Installed %s to %s\n", plugin.Name, wasmPath)
	fmt.Printf("Added to %s\n", pluginsConfig)
	return nil
}

func pluginList(args []string) error {
	pluginsConfig := "plugins.yaml"
	for i := 0; i < len(args); i++ {
		if args[i] == "--plugins-config" && i+1 < len(args) {
			pluginsConfig = args[i+1]
		}
	}

	var cfg PluginsConfig
	data, err := os.ReadFile(pluginsConfig)
	if err != nil {
		fmt.Println("No plugins installed.")
		return nil
	}
	yaml.Unmarshal(data, &cfg)

	all := append(cfg.Policies.Input, cfg.Policies.Output...)
	if len(all) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	fmt.Printf("%-25s %-10s %-12s %s\n", "NAME", "ACTION", "VERSION", "PATH")
	for _, p := range all {
		version := p.Version
		if version == "" {
			version = "unknown"
		}
		fmt.Printf("%-25s %-10s %-12s %s\n", p.Name, p.Action, version, p.Path)
	}
	return nil
}

func pluginOutdated(args []string) error {
	pluginsConfig := "plugins.yaml"
	for i := 0; i < len(args); i++ {
		if args[i] == "--plugins-config" && i+1 < len(args) {
			pluginsConfig = args[i+1]
		}
	}

	var cfg PluginsConfig
	data, err := os.ReadFile(pluginsConfig)
	if err != nil {
		fmt.Println("No plugins installed.")
		return nil
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing plugins config: %w", err)
	}

	reg, err := fetchRegistry("")
	if err != nil {
		return err
	}

	outdated := collectOutdatedPlugins(cfg, reg)
	if len(outdated) == 0 {
		fmt.Println("All installed plugins are up to date.")
		return nil
	}

	fmt.Printf("%-25s %-16s %s\n", "NAME", "INSTALLED", "LATEST")
	for _, plugin := range outdated {
		installed := plugin.InstalledVersion
		if installed == "" {
			installed = "unknown"
		}
		fmt.Printf("%-25s %-16s %s\n", plugin.Name, installed, plugin.LatestVersion)
	}
	return nil
}

func pluginRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aegisctl plugin remove <name>")
	}
	name := args[0]
	pluginsConfig := "plugins.yaml"

	var cfg PluginsConfig
	data, err := os.ReadFile(pluginsConfig)
	if err != nil {
		return fmt.Errorf("reading plugins config: %w", err)
	}
	yaml.Unmarshal(data, &cfg)

	var wasmPath string
	// Remove from input
	filtered := cfg.Policies.Input[:0]
	for _, p := range cfg.Policies.Input {
		if p.Name == name {
			wasmPath = p.Path
			continue
		}
		filtered = append(filtered, p)
	}
	cfg.Policies.Input = filtered

	// Remove from output
	filteredOut := cfg.Policies.Output[:0]
	for _, p := range cfg.Policies.Output {
		if p.Name == name {
			wasmPath = p.Path
			continue
		}
		filteredOut = append(filteredOut, p)
	}
	cfg.Policies.Output = filteredOut

	yamlData, _ := yaml.Marshal(cfg)
	os.WriteFile(pluginsConfig, yamlData, 0644)

	if wasmPath != "" {
		os.Remove(wasmPath)
		fmt.Printf("Removed %s and %s\n", name, wasmPath)
	} else {
		fmt.Printf("Plugin %s not found in config\n", name)
	}
	return nil
}

func containsTag(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

func collectOutdatedPlugins(cfg PluginsConfig, reg *Registry) []OutdatedPlugin {
	installed := append([]PluginPolicyEntry{}, cfg.Policies.Input...)
	installed = append(installed, cfg.Policies.Output...)

	registryByName := make(map[string]PluginEntry, len(reg.Plugins))
	for _, plugin := range reg.Plugins {
		registryByName[plugin.Name] = plugin
	}

	var outdated []OutdatedPlugin
	seen := make(map[string]bool)
	for _, plugin := range installed {
		if seen[plugin.Name] {
			continue
		}
		seen[plugin.Name] = true

		registryPlugin, ok := registryByName[plugin.Name]
		if !ok {
			continue
		}

		if plugin.Version == "" || compareVersion(plugin.Version, registryPlugin.Version) < 0 {
			outdated = append(outdated, OutdatedPlugin{
				Name:             plugin.Name,
				InstalledVersion: plugin.Version,
				LatestVersion:    registryPlugin.Version,
			})
		}
	}

	sort.Slice(outdated, func(i, j int) bool { return outdated[i].Name < outdated[j].Name })
	return outdated
}

func compareVersion(a, b string) int {
	if a == b {
		return 0
	}

	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		parts := strings.Split(v, ".")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			numeric := part
			for idx, ch := range part {
				if ch < '0' || ch > '9' {
					numeric = part[:idx]
					break
				}
			}
			if numeric == "" {
				out = append(out, 0)
				continue
			}
			n, err := strconv.Atoi(numeric)
			if err != nil {
				out = append(out, 0)
				continue
			}
			out = append(out, n)
		}
		return out
	}

	aa := parse(a)
	bb := parse(b)
	maxLen := len(aa)
	if len(bb) > maxLen {
		maxLen = len(bb)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aa) {
			av = aa[i]
		}
		if i < len(bb) {
			bv = bb[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
