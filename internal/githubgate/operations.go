package githubgate

import (
	"strings"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// RiskLevel indicates the danger level of a GitHub operation.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// operationInfo holds the classification for a known GitHub operation.
type operationInfo struct {
	Capability envelope.Capability
	Risk       RiskLevel
}

// knownOperations maps GitHub operation names to their capability and risk level.
var knownOperations = map[string]operationInfo{
	// Read operations — low risk
	"list_repos":         {envelope.CapRead, RiskLow},
	"get_repo":           {envelope.CapRead, RiskLow},
	"list_pull_requests": {envelope.CapRead, RiskLow},
	"get_pull_request":   {envelope.CapRead, RiskLow},
	"list_issues":        {envelope.CapRead, RiskLow},
	"get_issue":          {envelope.CapRead, RiskLow},
	"list_branches":      {envelope.CapRead, RiskLow},
	"get_commit":         {envelope.CapRead, RiskLow},
	"list_comments":      {envelope.CapRead, RiskLow},
	"search_code":        {envelope.CapRead, RiskLow},

	// Write operations — medium risk
	"create_issue":        {envelope.CapWrite, RiskMedium},
	"create_pull_request": {envelope.CapWrite, RiskMedium},
	"create_branch":       {envelope.CapWrite, RiskMedium},
	"create_comment":      {envelope.CapWrite, RiskMedium},
	"update_issue":        {envelope.CapWrite, RiskMedium},
	"update_pull_request": {envelope.CapWrite, RiskMedium},
	"create_release":      {envelope.CapWrite, RiskMedium},

	// Deploy operations — high risk
	"merge_pull_request": {envelope.CapDeploy, RiskHigh},
	"create_deployment":  {envelope.CapDeploy, RiskHigh},
	"push":               {envelope.CapDeploy, RiskHigh},

	// Delete operations — critical risk
	"delete_repo":    {envelope.CapDelete, RiskCritical},
	"delete_branch":  {envelope.CapDelete, RiskCritical},
	"delete_release": {envelope.CapDelete, RiskCritical},
	"delete_comment": {envelope.CapDelete, RiskCritical},
}

// ClassifyOperation returns the capability and risk level for a GitHub operation.
// Unknown operations default to write capability with medium risk.
func ClassifyOperation(operation string) (envelope.Capability, RiskLevel) {
	if info, ok := knownOperations[operation]; ok {
		return info.Capability, info.Risk
	}
	return inferCapability(operation), RiskMedium
}

// inferCapability guesses the capability from the operation name prefix.
func inferCapability(operation string) envelope.Capability {
	switch {
	case strings.HasPrefix(operation, "list_") || strings.HasPrefix(operation, "get_") || strings.HasPrefix(operation, "search_"):
		return envelope.CapRead
	case strings.HasPrefix(operation, "delete_"):
		return envelope.CapDelete
	case strings.HasPrefix(operation, "merge_") || strings.HasPrefix(operation, "deploy_"):
		return envelope.CapDeploy
	default:
		return envelope.CapWrite
	}
}
