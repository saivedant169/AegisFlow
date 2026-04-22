package main

import "testing"

func TestDefaultConfigPathUsesFallback(t *testing.T) {
	t.Setenv("AEGISFLOW_CONFIG", "")

	if got := defaultConfigPath(); got != defaultConfigFile {
		t.Fatalf("defaultConfigPath() = %q, want %q", got, defaultConfigFile)
	}
}

func TestDefaultConfigPathUsesEnvironment(t *testing.T) {
	t.Setenv("AEGISFLOW_CONFIG", " /etc/aegisflow/aegisflow.yaml ")

	if got := defaultConfigPath(); got != "/etc/aegisflow/aegisflow.yaml" {
		t.Fatalf("defaultConfigPath() = %q", got)
	}
}
