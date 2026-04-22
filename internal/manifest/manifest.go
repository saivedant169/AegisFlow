package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// TaskManifest declares what an agent session intends to do.
type TaskManifest struct {
	ID               string    `json:"id"`
	TaskID           string    `json:"task_id"` // ticket/issue/runbook reference
	Description      string    `json:"description"`
	Owner            string    `json:"owner"` // human sponsor
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	AllowedTools     []string  `json:"allowed_tools"`     // glob patterns
	AllowedResources []string  `json:"allowed_resources"` // glob patterns for targets
	AllowedProtocols []string  `json:"allowed_protocols"` // mcp, shell, sql, git, http
	AllowedVerbs     []string  `json:"allowed_verbs"`     // read, write, delete, deploy
	MaxActions       int       `json:"max_actions"`       // 0 = unlimited
	MaxBudget        float64   `json:"max_budget"`        // 0 = unlimited
	RiskTier         string    `json:"risk_tier"`         // low, medium, high
	ManifestHash     string    `json:"manifest_hash"`
	Active           bool      `json:"active"`
}

// ComputeHash returns a deterministic SHA-256 hash of the manifest's scope fields.
func (m *TaskManifest) ComputeHash() string {
	data, _ := json.Marshal(struct {
		TaskID           string   `json:"task_id"`
		AllowedTools     []string `json:"allowed_tools"`
		AllowedResources []string `json:"allowed_resources"`
		AllowedProtocols []string `json:"allowed_protocols"`
		AllowedVerbs     []string `json:"allowed_verbs"`
		MaxActions       int      `json:"max_actions"`
		RiskTier         string   `json:"risk_tier"`
	}{m.TaskID, m.AllowedTools, m.AllowedResources, m.AllowedProtocols, m.AllowedVerbs, m.MaxActions, m.RiskTier})
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
