package resource

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseGitHubResource extracts a typed Resource from GitHub tool operations.
// It infers the resource type and path from the tool name and parameters.
func ParseGitHubResource(tool, target string, params map[string]any) Resource {
	res := Resource{
		Provider:    "github",
		Environment: inferGitHubEnvironment(params),
		Sensitivity: SensitivityInternal,
		Properties:  make(map[string]string),
	}

	// Extract owner/repo from target (format: "owner/repo")
	parts := strings.SplitN(target, "/", 3)
	if len(parts) >= 2 {
		res.Path = []string{parts[0], parts[1]}
	} else if target != "" {
		res.Path = []string{target}
	}

	// Determine resource type from tool name
	toolLower := strings.ToLower(tool)
	switch {
	case strings.Contains(toolLower, "pull_request") || strings.Contains(toolLower, "pr"):
		res.Type = ResourcePR
		if pr, ok := params["pull_number"]; ok {
			res.Properties["pull_number"] = toString(pr)
		}
	case strings.Contains(toolLower, "workflow") || strings.Contains(toolLower, "action"):
		res.Type = ResourceWorkflow
	case strings.Contains(toolLower, "branch"):
		res.Type = ResourceBranch
		if branch, ok := params["branch"]; ok {
			res.Path = append(res.Path, toString(branch))
		}
	case strings.Contains(toolLower, "file") || strings.Contains(toolLower, "content"):
		res.Type = ResourceFile
		if filePath, ok := params["path"]; ok {
			res.Path = append(res.Path, toString(filePath))
		}
	default:
		res.Type = ResourceRepo
	}

	return res
}

// ParseSQLResource extracts a typed Resource from SQL operations.
// It parses the target for database/schema/table hierarchy.
func ParseSQLResource(tool, target string) Resource {
	res := Resource{
		Provider:    "postgres",
		Environment: "dev",
		Sensitivity: SensitivityConfidential,
		Properties:  make(map[string]string),
	}

	// Target format: "db.schema.table" or "db.table" or "table"
	parts := strings.Split(target, ".")
	switch len(parts) {
	case 3:
		res.Type = ResourceTable
		res.Path = parts
	case 2:
		res.Type = ResourceTable
		res.Path = parts
	case 1:
		if target != "" {
			res.Path = []string{target}
		}
		// Determine type from tool name.
		// "list_*" on a single target is a database-scope operation.
		toolLower := strings.ToLower(tool)
		switch {
		case strings.HasPrefix(toolLower, "list_"):
			res.Type = ResourceDatabase
		case strings.Contains(toolLower, "schema"):
			res.Type = ResourceSchema
		case strings.Contains(toolLower, "table"):
			res.Type = ResourceTable
		default:
			res.Type = ResourceDatabase
		}
	default:
		res.Type = ResourceDatabase
	}

	return res
}

// ParseShellResource extracts a typed Resource from shell commands.
// It identifies filesystem paths, network targets, and binary invocations.
func ParseShellResource(cmd, target string) Resource {
	res := Resource{
		Provider:    "shell",
		Environment: "dev",
		Sensitivity: SensitivityInternal,
		Properties:  make(map[string]string),
	}

	// Detect resource type from command
	cmdLower := strings.ToLower(cmd)
	switch {
	case strings.HasPrefix(target, "/") || strings.HasPrefix(target, "~") || strings.HasPrefix(target, "."):
		res.Type = ResourceFilesystem
		res.Path = strings.Split(strings.TrimPrefix(target, "/"), "/")
	case strings.Contains(cmdLower, "ssh") || strings.Contains(cmdLower, "curl") || strings.Contains(cmdLower, "wget"):
		res.Type = ResourceNetwork
		res.Path = []string{target}
	case strings.Contains(target, ":") || strings.Contains(target, ".") && !strings.HasPrefix(target, "/"):
		// Looks like a host:port or hostname
		res.Type = ResourceHost
		res.Path = []string{target}
	default:
		res.Type = ResourceFilesystem
		if target != "" {
			res.Path = []string{target}
		}
		if cmd != "" {
			res.Properties["command"] = cmd
		}
	}

	// Elevate sensitivity for dangerous commands
	switch {
	case strings.Contains(cmdLower, "rm ") || strings.Contains(cmdLower, "rm\t"):
		res.Sensitivity = SensitivityConfidential
	case strings.Contains(cmdLower, "sudo"):
		res.Sensitivity = SensitivitySecret
	case strings.Contains(cmdLower, "chmod") || strings.Contains(cmdLower, "chown"):
		res.Sensitivity = SensitivityConfidential
	}

	return res
}

// ParseHTTPResource extracts a typed Resource from HTTP API calls.
// It parses the URL to determine the service and endpoint.
func ParseHTTPResource(method, rawURL string) Resource {
	res := Resource{
		Provider:    "http",
		Environment: "dev",
		Sensitivity: SensitivityInternal,
		Properties:  make(map[string]string),
	}

	res.Properties["method"] = strings.ToUpper(method)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		res.Type = ResourceEndpoint
		res.Path = []string{rawURL}
		return res
	}

	res.Type = ResourceEndpoint
	host := parsed.Hostname()
	pathSegments := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	res.Path = append([]string{host}, pathSegments...)

	// Infer provider from known hosts
	switch {
	case strings.Contains(host, "github.com") || strings.Contains(host, "github"):
		res.Provider = "github"
	case strings.Contains(host, "aws.amazon.com") || strings.Contains(host, "amazonaws.com"):
		res.Provider = "aws"
	case strings.Contains(host, "stripe.com"):
		res.Provider = "stripe"
	case strings.Contains(host, "slack.com"):
		res.Provider = "slack"
	}

	// Infer environment from host/path
	switch {
	case strings.Contains(host, "prod") || strings.Contains(host, "production"):
		res.Environment = "prod"
	case strings.Contains(host, "staging") || strings.Contains(host, "stg"):
		res.Environment = "staging"
	}

	return res
}

// inferGitHubEnvironment guesses environment from branch or parameters.
func inferGitHubEnvironment(params map[string]any) string {
	if branch, ok := params["branch"]; ok {
		b := toString(branch)
		switch {
		case b == "main" || b == "master" || strings.HasPrefix(b, "release/"):
			return "prod"
		case strings.HasPrefix(b, "staging") || strings.HasPrefix(b, "stg"):
			return "staging"
		}
	}
	return "dev"
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprintf("%d", t)
	default:
		return fmt.Sprintf("%v", v)
	}
}
