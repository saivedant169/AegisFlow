package identity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newTestServer creates a chi router with identity admin routes registered,
// backed by a fresh in-memory store.
func newTestServer() (*httptest.Server, *Store) {
	store := NewStore()
	handler := NewAdminHandler(store)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), store
}

// postJSON sends a POST with JSON body and returns the response.
func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// --- Organization CRUD ---

func TestAdminCreateOrg(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "Acme"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var org Organization
	json.NewDecoder(resp.Body).Decode(&org)
	resp.Body.Close()
	if org.ID != "org-1" || org.Name != "Acme" {
		t.Fatalf("unexpected org: %+v", org)
	}
}

func TestAdminCreateOrgDuplicate(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "Acme"})
	resp := postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "Acme2"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateOrgInvalidJSON(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/v1/identity/orgs", "application/json", bytes.NewReader([]byte(`{bad`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

func TestAdminGetOrg(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "Acme"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/orgs/org-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var org Organization
	json.NewDecoder(resp.Body).Decode(&org)
	resp.Body.Close()
	if org.ID != "org-1" {
		t.Fatalf("expected org-1, got %s", org.ID)
	}
}

func TestAdminGetOrgNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/orgs/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminListOrgs(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "A"})
	postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-2", Name: "B"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/orgs")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var orgs []Organization
	json.NewDecoder(resp.Body).Decode(&orgs)
	resp.Body.Close()
	if len(orgs) != 2 {
		t.Fatalf("expected 2 orgs, got %d", len(orgs))
	}
}

func TestAdminDeleteOrg(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/orgs", Organization{ID: "org-1", Name: "Acme"})

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/orgs/org-1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify it's gone
	resp, _ = http.Get(srv.URL + "/admin/v1/identity/orgs/org-1")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminDeleteOrgNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/orgs/nope", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// --- Team CRUD ---

func TestAdminCreateTeam(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})

	resp := postJSON(t, srv.URL+"/admin/v1/identity/teams", Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateTeamBadParent(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/v1/identity/teams", Team{ID: "team-1", Name: "Backend", OrgID: "no-org"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for missing parent org, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetTeam(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/teams/team-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetTeamNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/teams/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminDeleteTeam(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/teams/team-1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminListTeamsForOrg(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "t1", Name: "Backend", OrgID: "org-1"})
	store.CreateTeam(Team{ID: "t2", Name: "Frontend", OrgID: "org-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/orgs/org-1/teams")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var teams []Team
	json.NewDecoder(resp.Body).Decode(&teams)
	resp.Body.Close()
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

// --- Project CRUD ---

func TestAdminCreateProject(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})

	resp := postJSON(t, srv.URL+"/admin/v1/identity/projects", Project{ID: "proj-1", Name: "API", TeamID: "team-1"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateProjectBadParent(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/v1/identity/projects", Project{ID: "proj-1", Name: "API", TeamID: "no-team"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetProject(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/projects/proj-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetProjectNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/projects/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminDeleteProject(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/projects/proj-1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminListProjectsForTeam(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "p1", Name: "API", TeamID: "team-1"})
	store.CreateProject(Project{ID: "p2", Name: "Web", TeamID: "team-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/teams/team-1/projects")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var projects []Project
	json.NewDecoder(resp.Body).Decode(&projects)
	resp.Body.Close()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

// --- Environment CRUD ---

func TestAdminCreateEnvironment(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})

	resp := postJSON(t, srv.URL+"/admin/v1/identity/environments", Environment{ID: "env-prod", Name: "Production", ProjectID: "proj-1", RiskTier: "high"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateEnvironmentBadParent(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/v1/identity/environments", Environment{ID: "env-1", Name: "Prod", ProjectID: "no-proj"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetEnvironment(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})
	store.CreateEnvironment(Environment{ID: "env-1", Name: "Prod", ProjectID: "proj-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/environments/env-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetEnvironmentNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/environments/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminDeleteEnvironment(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})
	store.CreateEnvironment(Environment{ID: "env-1", Name: "Prod", ProjectID: "proj-1"})

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/environments/env-1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminListEnvironmentsForProject(t *testing.T) {
	srv, store := newTestServer()
	defer srv.Close()
	store.CreateOrg(Organization{ID: "org-1", Name: "Acme"})
	store.CreateTeam(Team{ID: "team-1", Name: "Backend", OrgID: "org-1"})
	store.CreateProject(Project{ID: "proj-1", Name: "API", TeamID: "team-1"})
	store.CreateEnvironment(Environment{ID: "e1", Name: "Dev", ProjectID: "proj-1"})
	store.CreateEnvironment(Environment{ID: "e2", Name: "Prod", ProjectID: "proj-1"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/projects/proj-1/environments")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var envs []Environment
	json.NewDecoder(resp.Body).Decode(&envs)
	resp.Body.Close()
	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}
}

// --- Identity CRUD ---

func TestAdminCreateIdentity(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{
		ID: "agent-1", Type: "agent", Name: "CodeBot",
		Roles: []Role{{Name: "operator", Scope: "org", ScopeID: "org-1"}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateIdentityDuplicate(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "agent-1", Type: "agent", Name: "Bot"})
	resp := postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "agent-1", Type: "agent", Name: "Bot2"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminGetIdentity(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "agent-1", Type: "agent", Name: "Bot"})
	resp, _ := http.Get(srv.URL + "/admin/v1/identity/identities/agent-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var ident Identity
	json.NewDecoder(resp.Body).Decode(&ident)
	resp.Body.Close()
	if ident.ID != "agent-1" {
		t.Fatalf("expected agent-1, got %s", ident.ID)
	}
}

func TestAdminGetIdentityNotFound(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/identities/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminListIdentities(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "a1", Type: "agent", Name: "Bot1"})
	postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "a2", Type: "human", Name: "Alice"})

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/identities")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var idents []Identity
	json.NewDecoder(resp.Body).Decode(&idents)
	resp.Body.Close()
	if len(idents) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(idents))
	}
}

func TestAdminDeleteIdentity(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	postJSON(t, srv.URL+"/admin/v1/identity/identities", Identity{ID: "a1", Type: "agent", Name: "Bot"})

	req, _ := http.NewRequest("DELETE", srv.URL+"/admin/v1/identity/identities/a1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Confirm deleted
	resp, _ = http.Get(srv.URL + "/admin/v1/identity/identities/a1")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// --- Separation Rules ---

func TestAdminListSeparationRules(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/admin/v1/identity/separation-rules")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var rules []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	json.NewDecoder(resp.Body).Decode(&rules)
	resp.Body.Close()
	if len(rules) != 4 {
		t.Fatalf("expected 4 default rules, got %d", len(rules))
	}
	// Verify all rules have names
	for _, r := range rules {
		if r.Name == "" {
			t.Error("rule has empty name")
		}
		if r.Description == "" {
			t.Errorf("rule %q has empty description", r.Name)
		}
	}
}
