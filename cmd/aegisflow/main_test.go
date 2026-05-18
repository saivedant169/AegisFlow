package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

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

func TestVersionFlagLong(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperVersionLong$")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		t.Fatalf("helper process failed: %v", err)
	}

	expected := "aegisflow " + version + "\n"
	if got := stdout.String(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestHelperVersionLong(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Args = []string{"aegisflow", "-version"}
	main()
	os.Exit(0)
}

func TestVersionFlagShort(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperVersionShort$")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		t.Fatalf("helper process failed: %v", err)
	}

	expected := "aegisflow " + version + "\n"
	if got := stdout.String(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestHelperVersionShort(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Args = []string{"aegisflow", "-v"}
	main()
	os.Exit(0)
}
