package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCompletionCommands_NoDuplicatesOrBlanks(t *testing.T) {
	seen := make(map[string]bool, len(completionCommands))
	for _, c := range completionCommands {
		if strings.TrimSpace(c) == "" || strings.ContainsAny(c, " \t\n") {
			t.Fatalf("invalid completion entry %q", c)
		}
		if seen[c] {
			t.Fatalf("duplicate completion entry %q", c)
		}
		seen[c] = true
	}
}

func TestCompletionScripts_ListEveryCommand(t *testing.T) {
	scripts := map[string]string{
		"bash": bashCompletionScript(),
		"zsh":  zshCompletionScript(),
	}
	for shell, script := range scripts {
		for _, c := range completionCommands {
			if !strings.Contains(script, c) {
				t.Errorf("%s script is missing command %q", shell, c)
			}
		}
	}
}

// The scripts are only useful if the shell can parse them, so hand them to
// the real interpreters in no-exec mode. Skipped when a shell isn't installed.
func TestCompletionScripts_ValidSyntax(t *testing.T) {
	tests := []struct {
		shell  string
		script string
	}{
		{"bash", bashCompletionScript()},
		{"zsh", zshCompletionScript()},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			path, err := exec.LookPath(tt.shell)
			if err != nil {
				t.Skipf("%s not installed", tt.shell)
			}
			cmd := exec.Command(path, "-n")
			cmd.Stdin = strings.NewReader(tt.script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s rejected the script: %v\n%s", tt.shell, err, out)
			}
		})
	}
}

// Drives the generated bash function the way readline would and checks the
// candidates it offers.
func TestBashCompletion_Behavior(t *testing.T) {
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}

	tests := []struct {
		name     string
		words    string // contents of COMP_WORDS
		cword    int
		want     string // space-separated COMPREPLY
		wantNone bool
	}{
		{name: "prefix narrows to matches", words: `aegisctl po`, cword: 1, want: "policy-pack policies policy"},
		{name: "empty prefix lists all commands", words: `aegisctl ''`, cword: 1, want: strings.Join(completionCommands, " ")},
		{name: "exact single match", words: `aegisctl approve`, cword: 1, want: "approve"},
		{name: "no match", words: `aegisctl zzz`, cword: 1, wantNone: true},
		{name: "second argument is not completed", words: `aegisctl completion b`, cword: 2, wantNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := bashCompletionScript() +
				"\nCOMP_WORDS=(" + tt.words + ")\n" +
				"COMP_CWORD=" + strconv.Itoa(tt.cword) + "\n" +
				"COMPREPLY=(stale)\n" +
				"_aegisctl_completion\n" +
				`printf '%s\n' "${COMPREPLY[@]}"` + "\n"

			out, err := exec.Command(path, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("bash failed: %v\n%s", err, out)
			}
			got := strings.Fields(string(out))

			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("expected no candidates, got %v", got)
				}
				return
			}
			want := strings.Fields(tt.want)
			if len(got) != len(want) {
				t.Fatalf("candidates: got %v, want %v", got, want)
			}
			gotSet := make(map[string]bool, len(got))
			for _, g := range got {
				gotSet[g] = true
			}
			for _, w := range want {
				if !gotSet[w] {
					t.Fatalf("candidates: got %v, want %v", got, want)
				}
			}
		})
	}
}

// Use the real completion initializer for both supported Zsh loading methods.
// Replace only _describe, which normally needs an interactive completion context.
func TestZshCompletion_Behavior(t *testing.T) {
	path, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_aegisctl"), []byte(zshCompletionScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"source", "autoload"} {
		t.Run(mode, func(t *testing.T) {
			script := `
# Exclude broken third-party completion symlinks from the test environment.
fpath=( ${^fpath}/compinit(N:h) ${^fpath}/_main_complete(N:h) )
if [[ $2 == autoload ]]; then
	fpath=( "$1" $fpath )
fi
autoload -Uz compinit
compinit -D -i
if [[ $2 == source ]]; then
	source "$1/_aegisctl"
fi
[[ $_comps[aegisctl] == _aegisctl ]] || exit 10
_describe() { print -l -- "${(@P)2}"; }
# The first autoloaded call must produce candidates too.
CURRENT=2
_aegisctl
CURRENT=3
_aegisctl
CURRENT=4
_aegisctl
exit 0
`
			out, err := exec.Command(path, "-f", "-c", script, "zsh", dir, mode).CombinedOutput()
			if err != nil {
				t.Fatalf("zsh failed: %v\n%s", err, out)
			}
			want := strings.Join(completionCommands, "\n") + "\n"
			if string(out) != want {
				t.Fatalf("candidates: got %q, want %q", out, want)
			}
		})
	}
}

func TestCmdCompletion_Shells(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{"bash", bashCompletionScript()},
		{"zsh", zshCompletionScript()},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var err error
			got := captureStdout(t, func() { err = cmdCompletion([]string{tt.shell}) })
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("output does not match the %s script", tt.shell)
			}
		})
	}
}

func TestCmdCompletion_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no shell", nil, "usage: aegisctl completion"},
		{"unknown shell", []string{"fish"}, `"fish"`},
		{"extra argument", []string{"bash", "extra"}, "usage: aegisctl completion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() { err = cmdCompletion(tt.args) })
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
			if out != "" {
				t.Fatalf("nothing should be printed on error, got %q", out)
			}
		})
	}
}

func TestCmdCompletion_OutputFailure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed-output")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = original })
	for _, shell := range []string{"bash", "zsh"} {
		if err := cmdCompletion([]string{shell}); !errors.Is(err, os.ErrClosed) {
			t.Errorf("%s: expected closed-output error, got %v", shell, err)
		}
	}
}
