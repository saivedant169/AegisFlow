package identity

import (
	"testing"
)

func TestOrgTeamProjectHierarchy(t *testing.T) {
	s := NewStore()

	// Create org
	if err := s.CreateOrg(Organization{ID: "org-1", Name: "Acme Corp"}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// Duplicate org should fail
	if err := s.CreateOrg(Organization{ID: "org-1", Name: "Dup"}); err == nil {
		t.Fatal("expected duplicate org error")
	}

	// Create team under org
	if err := s.CreateTeam(Team{ID: "team-1", Name: "Platform", OrgID: "org-1"}); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	// Team with missing org should fail
	if err := s.CreateTeam(Team{ID: "team-2", Name: "Ghost", OrgID: "org-missing"}); err == nil {
		t.Fatal("expected missing org error")
	}

	// Create project under team
	if err := s.CreateProject(Project{ID: "proj-1", Name: "Gateway", TeamID: "team-1"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Project with missing team should fail
	if err := s.CreateProject(Project{ID: "proj-2", Name: "Ghost", TeamID: "team-missing"}); err == nil {
		t.Fatal("expected missing team error")
	}

	// Create environment under project
	if err := s.CreateEnvironment(Environment{ID: "env-prod", Name: "prod", ProjectID: "proj-1", RiskTier: "critical"}); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if err := s.CreateEnvironment(Environment{ID: "env-dev", Name: "dev", ProjectID: "proj-1", RiskTier: "low"}); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	// Environment with missing project should fail
	if err := s.CreateEnvironment(Environment{ID: "env-x", Name: "x", ProjectID: "proj-missing"}); err == nil {
		t.Fatal("expected missing project error")
	}

	// Hierarchy queries
	teams := s.GetTeamsForOrg("org-1")
	if len(teams) != 1 || teams[0].ID != "team-1" {
		t.Fatalf("GetTeamsForOrg: got %d teams", len(teams))
	}

	projects := s.GetProjectsForTeam("team-1")
	if len(projects) != 1 || projects[0].ID != "proj-1" {
		t.Fatalf("GetProjectsForTeam: got %d projects", len(projects))
	}

	envs := s.GetEnvironmentsForProject("proj-1")
	if len(envs) != 2 {
		t.Fatalf("GetEnvironmentsForProject: got %d environments, want 2", len(envs))
	}

	// Get individual entities
	org, err := s.GetOrg("org-1")
	if err != nil || org.Name != "Acme Corp" {
		t.Fatalf("GetOrg: %v, %+v", err, org)
	}

	team, err := s.GetTeam("team-1")
	if err != nil || team.Name != "Platform" {
		t.Fatalf("GetTeam: %v, %+v", err, team)
	}

	proj, err := s.GetProject("proj-1")
	if err != nil || proj.Name != "Gateway" {
		t.Fatalf("GetProject: %v, %+v", err, proj)
	}

	env, err := s.GetEnvironment("env-prod")
	if err != nil || env.RiskTier != "critical" {
		t.Fatalf("GetEnvironment: %v, %+v", err, env)
	}

	// Delete
	if err := s.DeleteEnvironment("env-prod"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if _, err := s.GetEnvironment("env-prod"); err == nil {
		t.Fatal("expected not-found after delete")
	}

	// List orgs
	orgs := s.ListOrgs()
	if len(orgs) != 1 {
		t.Fatalf("ListOrgs: got %d", len(orgs))
	}
}

func TestIdentityRoles(t *testing.T) {
	s := NewStore()

	ident := Identity{
		ID:     "user-1",
		Type:   "human",
		Name:   "Alice",
		OrgID:  "org-1",
		TeamID: "team-1",
		Roles: []Role{
			{Name: "admin", Scope: "org", ScopeID: "org-1"},
			{Name: "operator", Scope: "team", ScopeID: "team-1"},
			{Name: "viewer", Scope: "project", ScopeID: "proj-1"},
		},
	}

	if err := s.CreateIdentity(ident); err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}

	// Duplicate identity should fail
	if err := s.CreateIdentity(ident); err == nil {
		t.Fatal("expected duplicate identity error")
	}

	got, err := s.GetIdentity("user-1")
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if got.Name != "Alice" || len(got.Roles) != 3 {
		t.Fatalf("unexpected identity: %+v", got)
	}

	// List
	all := s.ListIdentities()
	if len(all) != 1 {
		t.Fatalf("ListIdentities: got %d", len(all))
	}

	// Delete
	if err := s.DeleteIdentity("user-1"); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	if _, err := s.GetIdentity("user-1"); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestScopedRoleLookup(t *testing.T) {
	s := NewStore()

	ident := Identity{
		ID:     "agent-1",
		Type:   "agent",
		Name:   "Deploy Bot",
		OrgID:  "org-1",
		TeamID: "team-1",
		Roles: []Role{
			{Name: "operator", Scope: "project", ScopeID: "proj-1"},
			{Name: "viewer", Scope: "project", ScopeID: "proj-2"},
			{Name: "admin", Scope: "org", ScopeID: "org-1"},
		},
	}
	if err := s.CreateIdentity(ident); err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}

	// Scoped lookup for project proj-1
	roles := s.GetRolesForScope("agent-1", "project", "proj-1")
	if len(roles) != 1 || roles[0].Name != "operator" {
		t.Fatalf("GetRolesForScope proj-1: got %+v", roles)
	}

	// Scoped lookup for org
	roles = s.GetRolesForScope("agent-1", "org", "org-1")
	if len(roles) != 1 || roles[0].Name != "admin" {
		t.Fatalf("GetRolesForScope org-1: got %+v", roles)
	}

	// Non-existent scope
	roles = s.GetRolesForScope("agent-1", "project", "proj-99")
	if len(roles) != 0 {
		t.Fatalf("expected empty roles for non-existent scope, got %+v", roles)
	}

	// Non-existent identity
	roles = s.GetRolesForScope("nobody", "org", "org-1")
	if len(roles) != 0 {
		t.Fatalf("expected empty roles for non-existent identity, got %+v", roles)
	}
}
