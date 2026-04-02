package main

import "testing"

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
