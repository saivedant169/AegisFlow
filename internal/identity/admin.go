package identity

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AdminHandler provides HTTP handlers for identity management endpoints.
type AdminHandler struct {
	store *Store
}

// NewAdminHandler creates an admin handler backed by the given store.
func NewAdminHandler(store *Store) *AdminHandler {
	return &AdminHandler{store: store}
}

// RegisterRoutes mounts identity management routes on the given router.
func (h *AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/admin/v1/identity/orgs", h.listOrgs)
	r.Post("/admin/v1/identity/orgs", h.createOrg)
	r.Get("/admin/v1/identity/orgs/{id}", h.getOrg)
	r.Delete("/admin/v1/identity/orgs/{id}", h.deleteOrg)
	r.Get("/admin/v1/identity/orgs/{id}/teams", h.listTeamsForOrg)

	r.Post("/admin/v1/identity/teams", h.createTeam)
	r.Get("/admin/v1/identity/teams/{id}", h.getTeam)
	r.Delete("/admin/v1/identity/teams/{id}", h.deleteTeam)
	r.Get("/admin/v1/identity/teams/{id}/projects", h.listProjectsForTeam)

	r.Post("/admin/v1/identity/projects", h.createProject)
	r.Get("/admin/v1/identity/projects/{id}", h.getProject)
	r.Delete("/admin/v1/identity/projects/{id}", h.deleteProject)
	r.Get("/admin/v1/identity/projects/{id}/environments", h.listEnvironmentsForProject)

	r.Post("/admin/v1/identity/environments", h.createEnvironment)
	r.Get("/admin/v1/identity/environments/{id}", h.getEnvironment)
	r.Delete("/admin/v1/identity/environments/{id}", h.deleteEnvironment)

	r.Get("/admin/v1/identity/identities", h.listIdentities)
	r.Post("/admin/v1/identity/identities", h.createIdentity)
	r.Get("/admin/v1/identity/identities/{id}", h.getIdentity)
	r.Delete("/admin/v1/identity/identities/{id}", h.deleteIdentity)

	r.Get("/admin/v1/identity/separation-rules", h.listSeparationRules)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Organizations ---

func (h *AdminHandler) listOrgs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListOrgs())
}

func (h *AdminHandler) createOrg(w http.ResponseWriter, r *http.Request) {
	var org Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.CreateOrg(org); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (h *AdminHandler) getOrg(w http.ResponseWriter, r *http.Request) {
	org, err := h.store.GetOrg(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *AdminHandler) deleteOrg(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteOrg(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) listTeamsForOrg(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetTeamsForOrg(chi.URLParam(r, "id")))
}

// --- Teams ---

func (h *AdminHandler) createTeam(w http.ResponseWriter, r *http.Request) {
	var team Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.CreateTeam(team); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

func (h *AdminHandler) getTeam(w http.ResponseWriter, r *http.Request) {
	team, err := h.store.GetTeam(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *AdminHandler) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteTeam(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) listProjectsForTeam(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetProjectsForTeam(chi.URLParam(r, "id")))
}

// --- Projects ---

func (h *AdminHandler) createProject(w http.ResponseWriter, r *http.Request) {
	var proj Project
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.CreateProject(proj); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, proj)
}

func (h *AdminHandler) getProject(w http.ResponseWriter, r *http.Request) {
	proj, err := h.store.GetProject(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func (h *AdminHandler) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteProject(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) listEnvironmentsForProject(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetEnvironmentsForProject(chi.URLParam(r, "id")))
}

// --- Environments ---

func (h *AdminHandler) createEnvironment(w http.ResponseWriter, r *http.Request) {
	var env Environment
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.CreateEnvironment(env); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, env)
}

func (h *AdminHandler) getEnvironment(w http.ResponseWriter, r *http.Request) {
	env, err := h.store.GetEnvironment(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (h *AdminHandler) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteEnvironment(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Identities ---

func (h *AdminHandler) listIdentities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListIdentities())
}

func (h *AdminHandler) createIdentity(w http.ResponseWriter, r *http.Request) {
	var ident Identity
	if err := json.NewDecoder(r.Body).Decode(&ident); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.CreateIdentity(ident); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ident)
}

func (h *AdminHandler) getIdentity(w http.ResponseWriter, r *http.Request) {
	ident, err := h.store.GetIdentity(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ident)
}

func (h *AdminHandler) deleteIdentity(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteIdentity(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Separation Rules ---

func (h *AdminHandler) listSeparationRules(w http.ResponseWriter, r *http.Request) {
	rules := DefaultRules()
	type ruleInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]ruleInfo, len(rules))
	for i, rule := range rules {
		out[i] = ruleInfo{Name: rule.Name, Description: rule.Description}
	}
	writeJSON(w, http.StatusOK, out)
}
