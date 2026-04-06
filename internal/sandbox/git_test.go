package sandbox

import "testing"

func TestGitSandbox_AllowedRepos(t *testing.T) {
	s := &GitSandbox{
		AllowedRepos: []string{"myorg/*", "otherorg/specific-repo"},
	}

	tests := []struct {
		repo string
		want bool
	}{
		{"myorg/frontend", false},
		{"myorg/backend", false},
		{"otherorg/specific-repo", false},
		{"otherorg/other-repo", true},
		{"evil/repo", true},
	}

	for _, tt := range tests {
		v := s.Validate("push", tt.repo, "feature/x", "")
		got := v != nil
		if got != tt.want {
			t.Errorf("AllowedRepos repo=%q: got violation=%v, want %v", tt.repo, got, tt.want)
		}
	}
}

func TestGitSandbox_BlockForcePush(t *testing.T) {
	s := &GitSandbox{BlockForcePush: true}

	tests := []struct {
		op   string
		want bool
	}{
		{"force_push", true},
		{"push", false},
		{"merge", false},
		{"commit", false},
	}

	for _, tt := range tests {
		v := s.Validate(tt.op, "org/repo", "main", "")
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockForcePush op=%q: got violation=%v, want %v", tt.op, got, tt.want)
		}
	}
}

func TestGitSandbox_BlockMainMerge(t *testing.T) {
	s := &GitSandbox{BlockMainMerge: true}

	tests := []struct {
		op     string
		branch string
		want   bool
	}{
		{"merge", "main", true},
		{"merge", "master", true},
		{"merge", "develop", false},
		{"push", "main", false}, // only merge is blocked
		{"merge", "feature/x", false},
	}

	for _, tt := range tests {
		v := s.Validate(tt.op, "org/repo", tt.branch, "")
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockMainMerge op=%q branch=%q: got violation=%v, want %v", tt.op, tt.branch, got, tt.want)
		}
	}
}

func TestGitSandbox_AllowedBranches(t *testing.T) {
	s := &GitSandbox{
		AllowedBranches: []string{"agent/*", "bot/*"},
	}

	tests := []struct {
		op     string
		branch string
		want   bool
	}{
		{"push", "agent/feature-1", false},
		{"push", "bot/fix-1", false},
		{"push", "main", true},
		{"push", "develop", true},
		{"commit", "agent/x", false},
		// read operations should not be restricted
		{"clone", "main", false},
		{"fetch", "main", false},
	}

	for _, tt := range tests {
		v := s.Validate(tt.op, "org/repo", tt.branch, "")
		got := v != nil
		if got != tt.want {
			t.Errorf("AllowedBranches op=%q branch=%q: got violation=%v, want %v", tt.op, tt.branch, got, tt.want)
		}
	}
}

func TestGitSandbox_BlockWorkflowEdit(t *testing.T) {
	s := &GitSandbox{BlockWorkflowEdit: true}

	tests := []struct {
		filePath string
		want     bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/deploy.yaml", true},
		{".github/actions/custom/action.yml", true},
		{"src/main.go", false},
		{"README.md", false},
		{".github/CODEOWNERS", false},
	}

	for _, tt := range tests {
		v := s.Validate("edit", "org/repo", "feature/x", tt.filePath)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockWorkflowEdit file=%q: got violation=%v, want %v", tt.filePath, got, tt.want)
		}
	}
}

func TestGitSandbox_ProtectedPaths(t *testing.T) {
	s := &GitSandbox{
		ProtectedPaths: []string{".github/workflows/*", "Makefile", "go.mod"},
	}

	tests := []struct {
		filePath string
		want     bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/deploy.yaml", true},
		{"Makefile", true},
		{"go.mod", true},
		{"src/main.go", false},
		{"internal/handler.go", false},
	}

	for _, tt := range tests {
		v := s.Validate("edit", "org/repo", "feature/x", tt.filePath)
		got := v != nil
		if got != tt.want {
			t.Errorf("ProtectedPaths file=%q: got violation=%v, want %v", tt.filePath, got, tt.want)
		}
	}
}

func TestGitSandbox_EmptySandbox(t *testing.T) {
	s := &GitSandbox{}

	v := s.Validate("force_push", "evil/repo", "main", ".github/workflows/ci.yml")
	if v != nil {
		t.Errorf("empty sandbox should allow everything, got: %v", v)
	}
}
