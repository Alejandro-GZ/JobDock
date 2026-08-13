package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

const (
	scopeNodesRead     = "nodes:read"
	scopeJobsRead      = "jobs:read"
	scopeJobsWrite     = "jobs:write"
	scopeLogsRead      = "logs:read"
	scopeArtifactsRead = "artifacts:read"
)

var personalAccessTokenScopes = []string{scopeNodesRead, scopeJobsRead, scopeJobsWrite, scopeLogsRead, scopeArtifactsRead}

func (a *API) listPersonalAccessTokens(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListPersonalAccessTokens(r.Context(), currentUser(r).ID)
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "available_scopes": personalAccessTokenScopes})
}

func (a *API) createPersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 128 {
		writeProblem(w, 422, "invalid_token_name", "Token name must contain between 1 and 128 characters")
		return
	}
	if len(body.Scopes) == 0 {
		writeProblem(w, 422, "invalid_token_scopes", "At least one scope is required")
		return
	}
	seen := map[string]bool{}
	for _, scope := range body.Scopes {
		if !slices.Contains(personalAccessTokenScopes, scope) || seen[scope] {
			writeProblem(w, 422, "invalid_token_scopes", "Scopes must be unique supported values")
			return
		}
		seen[scope] = true
	}
	if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now().UTC()) {
		writeProblem(w, 422, "invalid_token_expiry", "Token expiry must be in the future")
		return
	}
	secret := "jdp_" + ids.Token(32)
	now := time.Now().UTC()
	item := store.PersonalAccessToken{ID: ids.New(), UserID: currentUser(r).ID, Name: body.Name, Prefix: secret[:12], Scopes: body.Scopes, ExpiresAt: body.ExpiresAt, CreatedAt: now}
	if err := a.store.CreatePersonalAccessToken(r.Context(), item, auth.TokenHash(secret)); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), item.UserID, "auth.pat.create", "personal_access_token", item.ID, map[string]any{"name": item.Name, "scopes": item.Scopes, "expires_at": item.ExpiresAt})
	writeJSON(w, http.StatusCreated, map[string]any{"token": secret, "personal_access_token": item})
}

func (a *API) revokePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	if err := a.store.RevokePersonalAccessToken(r.Context(), user.ID, id, time.Now().UTC()); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "auth.pat.revoke", "personal_access_token", id, map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func hasScope(scopes []string, required string) bool { return slices.Contains(scopes, required) }
