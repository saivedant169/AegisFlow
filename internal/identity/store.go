package identity

import (
	"fmt"
	"sync"
)

// Store is a thread-safe in-memory store for the identity hierarchy.
type Store struct {
	mu           sync.RWMutex
	orgs         map[string]*Organization
	teams        map[string]*Team
	projects     map[string]*Project
	environments map[string]*Environment
	identities   map[string]*Identity
}

// NewStore returns an empty identity store.
func NewStore() *Store {
	return &Store{
		orgs:         make(map[string]*Organization),
		teams:        make(map[string]*Team),
		projects:     make(map[string]*Project),
		environments: make(map[string]*Environment),
		identities:   make(map[string]*Identity),
	}
}

// --- Organizations ---

func (s *Store) CreateOrg(org Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orgs[org.ID]; exists {
		return fmt.Errorf("organization %q already exists", org.ID)
	}
	s.orgs[org.ID] = &org
	return nil
}

func (s *Store) GetOrg(id string) (*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	org, ok := s.orgs[id]
	if !ok {
		return nil, fmt.Errorf("organization %q not found", id)
	}
	return org, nil
}

func (s *Store) ListOrgs() []*Organization {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Organization, 0, len(s.orgs))
	for _, org := range s.orgs {
		result = append(result, org)
	}
	return result
}

func (s *Store) DeleteOrg(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orgs[id]; !exists {
		return fmt.Errorf("organization %q not found", id)
	}
	delete(s.orgs, id)
	return nil
}

// --- Teams ---

func (s *Store) CreateTeam(team Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orgs[team.OrgID]; !exists {
		return fmt.Errorf("parent organization %q not found", team.OrgID)
	}
	if _, exists := s.teams[team.ID]; exists {
		return fmt.Errorf("team %q already exists", team.ID)
	}
	s.teams[team.ID] = &team
	return nil
}

func (s *Store) GetTeam(id string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[id]
	if !ok {
		return nil, fmt.Errorf("team %q not found", id)
	}
	return team, nil
}

func (s *Store) GetTeamsForOrg(orgID string) []*Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Team
	for _, t := range s.teams {
		if t.OrgID == orgID {
			result = append(result, t)
		}
	}
	return result
}

func (s *Store) DeleteTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.teams[id]; !exists {
		return fmt.Errorf("team %q not found", id)
	}
	delete(s.teams, id)
	return nil
}

// --- Projects ---

func (s *Store) CreateProject(proj Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.teams[proj.TeamID]; !exists {
		return fmt.Errorf("parent team %q not found", proj.TeamID)
	}
	if _, exists := s.projects[proj.ID]; exists {
		return fmt.Errorf("project %q already exists", proj.ID)
	}
	s.projects[proj.ID] = &proj
	return nil
}

func (s *Store) GetProject(id string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proj, ok := s.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return proj, nil
}

func (s *Store) GetProjectsForTeam(teamID string) []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Project
	for _, p := range s.projects {
		if p.TeamID == teamID {
			result = append(result, p)
		}
	}
	return result
}

func (s *Store) DeleteProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[id]; !exists {
		return fmt.Errorf("project %q not found", id)
	}
	delete(s.projects, id)
	return nil
}

// --- Environments ---

func (s *Store) CreateEnvironment(env Environment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[env.ProjectID]; !exists {
		return fmt.Errorf("parent project %q not found", env.ProjectID)
	}
	if _, exists := s.environments[env.ID]; exists {
		return fmt.Errorf("environment %q already exists", env.ID)
	}
	s.environments[env.ID] = &env
	return nil
}

func (s *Store) GetEnvironment(id string) (*Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	env, ok := s.environments[id]
	if !ok {
		return nil, fmt.Errorf("environment %q not found", id)
	}
	return env, nil
}

func (s *Store) GetEnvironmentsForProject(projectID string) []*Environment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Environment
	for _, e := range s.environments {
		if e.ProjectID == projectID {
			result = append(result, e)
		}
	}
	return result
}

func (s *Store) DeleteEnvironment(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.environments[id]; !exists {
		return fmt.Errorf("environment %q not found", id)
	}
	delete(s.environments, id)
	return nil
}

// --- Identities ---

func (s *Store) CreateIdentity(ident Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.identities[ident.ID]; exists {
		return fmt.Errorf("identity %q already exists", ident.ID)
	}
	s.identities[ident.ID] = &ident
	return nil
}

func (s *Store) GetIdentity(id string) (*Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ident, ok := s.identities[id]
	if !ok {
		return nil, fmt.Errorf("identity %q not found", id)
	}
	return ident, nil
}

func (s *Store) ListIdentities() []*Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Identity, 0, len(s.identities))
	for _, ident := range s.identities {
		result = append(result, ident)
	}
	return result
}

func (s *Store) DeleteIdentity(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.identities[id]; !exists {
		return fmt.Errorf("identity %q not found", id)
	}
	delete(s.identities, id)
	return nil
}

// GetRolesForScope returns all roles an identity holds at the given scope type
// and scope ID.
func (s *Store) GetRolesForScope(identityID, scope, scopeID string) []Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ident, ok := s.identities[identityID]
	if !ok {
		return nil
	}
	var roles []Role
	for _, r := range ident.Roles {
		if r.Scope == scope && r.ScopeID == scopeID {
			roles = append(roles, r)
		}
	}
	return roles
}
