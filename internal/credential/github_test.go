package credential

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGithubRepoName(t *testing.T) {
	cases := map[string]string{
		"acme/widgets":   "widgets",
		"widgets":        "widgets",
		"org/team/repo":  "repo",
		" acme/widgets ": "widgets",
		"":               "",
		"acme/*":         "",
		"acme/widgets?":  "",
	}
	for in, want := range cases {
		if got := githubRepoName(in); got != want {
			t.Errorf("githubRepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGithubTokenRequestBody_Scoped(t *testing.T) {
	body, ok := githubTokenRequestBody(CredentialRequest{Target: "acme/widgets", Capability: "write"})
	if !ok {
		t.Fatal("expected a scoped body for a known capability")
	}
	var parsed struct {
		Repositories []string          `json:"repositories"`
		Permissions  map[string]string `json:"permissions"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if len(parsed.Repositories) != 1 || parsed.Repositories[0] != "widgets" {
		t.Fatalf("expected repositories=[widgets], got %v", parsed.Repositories)
	}
	if parsed.Permissions["pull_requests"] != "write" || parsed.Permissions["contents"] != "write" {
		t.Fatalf("expected write permissions, got %v", parsed.Permissions)
	}
}

func TestGithubTokenRequestBody_UnknownCapabilityIsUnscoped(t *testing.T) {
	if _, ok := githubTokenRequestBody(CredentialRequest{Target: "acme/widgets", Capability: "telepathy"}); ok {
		t.Fatal("an unknown capability should not produce a scoped body")
	}
}

func TestGitHubBrokerIssueSendsScopedBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token":      "ghs_faketoken",
			"expires_at": time.Now().Add(time.Hour).UTC(),
		})
	}))
	defer srv.Close()

	b := NewGitHubAppBroker("gh", 1, "", 99, time.Hour,
		WithGitHubBaseURL(srv.URL),
		WithJWTFunc(func() (string, error) { return "fake-jwt", nil }),
	)

	cred, err := b.Issue(context.Background(), CredentialRequest{TaskID: "t", Target: "acme/widgets", Capability: "read"})
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if cred.Token != "ghs_faketoken" {
		t.Fatalf("unexpected token: %s", cred.Token)
	}
	if len(gotBody) == 0 {
		t.Fatal("broker sent an empty (unscoped) body to GitHub")
	}
	var parsed map[string]any
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("sent body not JSON: %v", err)
	}
	if _, ok := parsed["permissions"]; !ok {
		t.Fatalf("scoped request must include permissions, got %s", string(gotBody))
	}
}
