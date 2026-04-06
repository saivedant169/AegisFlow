package resource

import "strings"

// Resource represents a typed, hierarchical resource being accessed.
type Resource struct {
	Type        ResourceType      `json:"type" yaml:"type"`
	Provider    string            `json:"provider" yaml:"provider"`       // github, postgres, aws, shell
	Environment string            `json:"environment" yaml:"environment"` // dev, staging, prod
	Path        []string          `json:"path" yaml:"path"`              // hierarchical: [org, repo, branch] or [db, schema, table]
	Sensitivity Sensitivity       `json:"sensitivity" yaml:"sensitivity"`
	Properties  map[string]string `json:"properties,omitempty" yaml:"properties,omitempty"`
}

// ResourceType classifies the kind of resource being accessed.
type ResourceType string

const (
	ResourceRepo       ResourceType = "repo"
	ResourceBranch     ResourceType = "branch"
	ResourceFile       ResourceType = "file"
	ResourcePR         ResourceType = "pull_request"
	ResourceWorkflow   ResourceType = "workflow"
	ResourceDatabase   ResourceType = "database"
	ResourceSchema     ResourceType = "schema"
	ResourceTable      ResourceType = "table"
	ResourceHost       ResourceType = "host"
	ResourceFilesystem ResourceType = "filesystem"
	ResourceNetwork    ResourceType = "network"
	ResourceEndpoint   ResourceType = "endpoint"
	ResourceBucket     ResourceType = "bucket"
	ResourceNamespace  ResourceType = "namespace"
	ResourceCluster    ResourceType = "cluster"
	ResourceAccount    ResourceType = "account"
)

// Sensitivity indicates the data classification level.
type Sensitivity string

const (
	SensitivityPublic       Sensitivity = "public"
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivitySecret       Sensitivity = "secret"
)

// sensitivityRank returns a numeric rank for ordering sensitivity levels.
func sensitivityRank(s Sensitivity) int {
	switch s {
	case SensitivityPublic:
		return 0
	case SensitivityInternal:
		return 1
	case SensitivityConfidential:
		return 2
	case SensitivitySecret:
		return 3
	default:
		return -1
	}
}

// Verb represents the action being taken on a resource.
type Verb string

const (
	VerbRead    Verb = "read"
	VerbCreate  Verb = "create"
	VerbUpdate  Verb = "update"
	VerbDelete  Verb = "delete"
	VerbDeploy  Verb = "deploy"
	VerbApprove Verb = "approve"
	VerbExecute Verb = "execute"
	VerbGrant   Verb = "grant"
)

// PathString returns the path segments joined by "/".
func (r *Resource) PathString() string {
	return strings.Join(r.Path, "/")
}
