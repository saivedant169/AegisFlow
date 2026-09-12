package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"1.10.0", "1.2.0", 1},
		{"v1.2.3", "1.2.4", -1},
		{"1.2.3-beta1", "1.2.3", 0},
	}
	for _, tc := range tests {
		if got := compareVersion(tc.a, tc.b); got != tc.want {
			t.Fatalf("compareVersion(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCollectOutdatedPlugins(t *testing.T) {
	cfg := PluginsConfig{}
	cfg.Policies.Input = []PluginPolicyEntry{
		{Name: "pii", Version: "0.9.0"},
		{Name: "stable", Version: "1.1.0"},
	}
	cfg.Policies.Output = []PluginPolicyEntry{
		{Name: "unknown-version"},
	}
	reg := &Registry{
		Plugins: []PluginEntry{
			{Name: "pii", Version: "1.0.0"},
			{Name: "stable", Version: "1.1.0"},
			{Name: "unknown-version", Version: "0.5.0"},
		},
	}

	outdated := collectOutdatedPlugins(cfg, reg)
	if len(outdated) != 2 {
		t.Fatalf("expected 2 outdated plugins, got %d", len(outdated))
	}
	if outdated[0].Name != "pii" || outdated[0].InstalledVersion != "0.9.0" || outdated[0].LatestVersion != "1.0.0" {
		t.Fatalf("unexpected first outdated plugin: %+v", outdated[0])
	}
	if outdated[1].Name != "unknown-version" || outdated[1].InstalledVersion != "" || outdated[1].LatestVersion != "0.5.0" {
		t.Fatalf("unexpected second outdated plugin: %+v", outdated[1])
	}
}

func TestPluginCommandsRejectMalformedConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	original := []byte("policies: [invalid")
	if err := os.WriteFile("plugins.yaml", original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func() error{
		func() error { return pluginList(nil) },
		func() error { return pluginRemove([]string{"demo"}) },
		func() error { return pluginInstall([]string{"demo"}) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "parsing plugins config") {
			t.Fatalf("expected parse failure: %v", err)
		}
		data, err := os.ReadFile("plugins.yaml")
		if err != nil || string(data) != string(original) {
			t.Fatal("malformed config changed")
		}
	}
}
func TestPluginConfigIOFailures(t *testing.T) {
	dir := t.TempDir()
	if _, err := readPluginsConfig(dir); err == nil {
		t.Fatal("directory read claimed empty config")
	}
	path := filepath.Join(dir, "occupied")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePluginsConfig(path, PluginsConfig{}); err == nil {
		t.Fatal("replacement of directory claimed success")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}
func TestPluginRemoveReportsPartialFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("occupied", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("occupied/keep", []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := PluginsConfig{}
	cfg.Policies.Input = []PluginPolicyEntry{{Name: "demo", Path: "occupied"}}
	if err := writePluginsConfig("plugins.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	if err := pluginRemove([]string{"demo"}); err == nil || !strings.Contains(err.Error(), "file removal failed") {
		t.Fatalf("expected partial failure: %v", err)
	}
	if _, err := os.Stat("occupied/keep"); err != nil {
		t.Fatal("unrelated file removed")
	}
}

func TestPluginInstallRollsBackOnConfigFailure(t *testing.T) {
	for _, existing := range []bool{true, false} {
		dir := t.TempDir()
		wasm := filepath.Join(dir, "demo.wasm")
		if existing {
			if err := os.WriteFile(wasm, []byte("previous"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		err := persistPluginInstall(wasm, []byte("new"), filepath.Join(dir, "missing", "plugins.yaml"), PluginsConfig{})
		if err == nil {
			t.Fatal("failed config write claimed install success")
		}
		got, readErr := os.ReadFile(wasm)
		if existing && (readErr != nil || string(got) != "previous") {
			t.Fatalf("previous binary lost: %s %v", got, readErr)
		}
		if !existing && !os.IsNotExist(readErr) {
			t.Fatalf("orphan binary remains: %v", readErr)
		}
	}
}
