package sandbox

import (
	"path/filepath"
	"strings"
)

// GitSandbox enforces constraints on Git and GitHub operations.
type GitSandbox struct {
	AllowedRepos      []string `yaml:"allowed_repos" json:"allowed_repos"`
	AllowedBranches   []string `yaml:"allowed_branches" json:"allowed_branches"`
	ProtectedPaths    []string `yaml:"protected_paths" json:"protected_paths"`
	BlockForcePush    bool     `yaml:"block_force_push" json:"block_force_push"`
	BlockMainMerge    bool     `yaml:"block_main_merge" json:"block_main_merge"`
	BlockWorkflowEdit bool     `yaml:"block_workflow_edit" json:"block_workflow_edit"`
}

// Validate checks a Git operation against sandbox constraints and returns the
// first violation found, or nil if the operation is allowed.
//
// Parameters:
//   - operation: the git operation (e.g. "push", "merge", "commit", "force_push", "edit")
//   - repo: the repository identifier (e.g. "org/repo")
//   - branch: the target branch
//   - filePath: a file being modified (may be empty)
func (s *GitSandbox) Validate(operation, repo, branch, filePath string) *SandboxViolation {
	// Check allowed repos.
	if len(s.AllowedRepos) > 0 && repo != "" {
		allowed := false
		for _, pattern := range s.AllowedRepos {
			if matchRepoPattern(repo, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "git",
				Rule:        "allowed_repo",
				Message:     "repository not in allowlist: " + repo,
				Severity:    "block",
			}
		}
	}

	// Block force push.
	if s.BlockForcePush && operation == "force_push" {
		return &SandboxViolation{
			SandboxType: "git",
			Rule:        "block_force_push",
			Message:     "force push is blocked",
			Severity:    "block",
		}
	}

	// Block merge to main/master.
	if s.BlockMainMerge && operation == "merge" && isMainBranch(branch) {
		return &SandboxViolation{
			SandboxType: "git",
			Rule:        "block_main_merge",
			Message:     "merge to " + branch + " is blocked",
			Severity:    "block",
		}
	}

	// Check allowed branches for write operations.
	if len(s.AllowedBranches) > 0 && branch != "" && isWriteOperation(operation) {
		allowed := false
		for _, pattern := range s.AllowedBranches {
			if matchBranchPattern(branch, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "git",
				Rule:        "allowed_branch",
				Message:     "branch not in allowlist for write operations: " + branch,
				Severity:    "block",
			}
		}
	}

	// Block workflow file edits.
	if s.BlockWorkflowEdit && filePath != "" && isWorkflowFile(filePath) {
		return &SandboxViolation{
			SandboxType: "git",
			Rule:        "block_workflow_edit",
			Message:     "editing workflow files is blocked: " + filePath,
			Severity:    "block",
		}
	}

	// Check protected paths.
	if filePath != "" && len(s.ProtectedPaths) > 0 {
		for _, pattern := range s.ProtectedPaths {
			if matchProtectedPath(filePath, pattern) {
				return &SandboxViolation{
					SandboxType: "git",
					Rule:        "protected_path",
					Message:     "file is in a protected path: " + filePath,
					Severity:    "block",
				}
			}
		}
	}

	return nil
}

func isMainBranch(branch string) bool {
	return branch == "main" || branch == "master"
}

func isWriteOperation(op string) bool {
	switch op {
	case "push", "force_push", "merge", "commit", "edit":
		return true
	}
	return false
}

func isWorkflowFile(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.HasPrefix(normalized, ".github/workflows/") ||
		strings.HasPrefix(normalized, ".github/actions/")
}

// matchRepoPattern matches a repo (e.g. "org/repo") against a glob pattern.
func matchRepoPattern(repo, pattern string) bool {
	matched, _ := filepath.Match(pattern, repo)
	return matched
}

// matchBranchPattern matches a branch name against a glob pattern (e.g. "agent/*").
func matchBranchPattern(branch, pattern string) bool {
	matched, _ := filepath.Match(pattern, branch)
	return matched
}

// matchProtectedPath checks if a file path matches a protected path pattern.
func matchProtectedPath(filePath, pattern string) bool {
	normalizedPath := filepath.ToSlash(filePath)
	normalizedPattern := filepath.ToSlash(pattern)
	matched, _ := filepath.Match(normalizedPattern, normalizedPath)
	if matched {
		return true
	}
	// Also check prefix match for directory patterns.
	if strings.HasSuffix(normalizedPattern, "/*") {
		dir := strings.TrimSuffix(normalizedPattern, "/*")
		if strings.HasPrefix(normalizedPath, dir+"/") {
			return true
		}
	}
	return false
}
