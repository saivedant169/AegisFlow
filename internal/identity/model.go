package identity

// Organization is the top-level tenant in the hierarchy.
type Organization struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name" yaml:"name"`
}

// Team belongs to an Organization.
type Team struct {
	ID    string `json:"id" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	OrgID string `json:"org_id" yaml:"org_id"`
}

// Project belongs to a Team.
type Project struct {
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name" yaml:"name"`
	TeamID string `json:"team_id" yaml:"team_id"`
}

// Environment belongs to a Project and carries a risk tier.
type Environment struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"` // dev, staging, prod
	ProjectID string `json:"project_id" yaml:"project_id"`
	RiskTier  string `json:"risk_tier" yaml:"risk_tier"` // low, medium, high, critical
}

// Identity represents a human, agent, or service account.
type Identity struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // human, agent, service
	Name   string `json:"name"`
	OrgID  string `json:"org_id"`
	TeamID string `json:"team_id"`
	Roles  []Role `json:"roles"`
}

// Role is a scoped permission grant.
type Role struct {
	Name    string `json:"name"`     // admin, operator, viewer, policy_author, approver
	Scope   string `json:"scope"`    // org, team, project, environment
	ScopeID string `json:"scope_id"` // ID of the scope entity
}
