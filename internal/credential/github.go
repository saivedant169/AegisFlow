package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GitHubAppBroker issues short-lived installation access tokens via the GitHub App API.
type GitHubAppBroker struct {
	name          string
	appID         int64
	keyPath       string
	installID     int64
	defaultTTL    time.Duration
	client        *http.Client
	baseURL       string // defaults to "https://api.github.com"; override for testing
	jwtFn         func() (string, error) // produces a JWT; injectable for testing

	mu      sync.Mutex
	revoked map[string]bool
}

// GitHubAppBrokerOption configures a GitHubAppBroker.
type GitHubAppBrokerOption func(*GitHubAppBroker)

// WithGitHubBaseURL overrides the GitHub API base URL (for testing with httptest).
func WithGitHubBaseURL(url string) GitHubAppBrokerOption {
	return func(b *GitHubAppBroker) {
		b.baseURL = url
	}
}

// WithJWTFunc overrides the JWT generation function (for testing without a real private key).
func WithJWTFunc(fn func() (string, error)) GitHubAppBrokerOption {
	return func(b *GitHubAppBroker) {
		b.jwtFn = fn
	}
}

// NewGitHubAppBroker creates a new GitHub App credential broker.
func NewGitHubAppBroker(name string, appID int64, keyPath string, installID int64, defaultTTL time.Duration, opts ...GitHubAppBrokerOption) *GitHubAppBroker {
	b := &GitHubAppBroker{
		name:       name,
		appID:      appID,
		keyPath:    keyPath,
		installID:  installID,
		defaultTTL: defaultTTL,
		client:     &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.github.com",
		revoked:    make(map[string]bool),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Name returns the broker name.
func (b *GitHubAppBroker) Name() string {
	return b.name
}

// installationTokenResponse represents the GitHub API response for creating
// an installation access token.
type installationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Issue creates an installation access token via the GitHub App API.
func (b *GitHubAppBroker) Issue(ctx context.Context, req CredentialRequest) (*Credential, error) {
	jwt, err := b.getJWT()
	if err != nil {
		return nil, fmt.Errorf("generating JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", b.baseURL, b.installID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+jwt)
	httpReq.Header.Set("Accept", "application/vnd.github+json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling GitHub API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp installationTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := tokenResp.ExpiresAt
	// GitHub tokens last up to 1 hour; cap to requested TTL if shorter.
	if req.TTL > 0 && now.Add(req.TTL).Before(expiresAt) {
		expiresAt = now.Add(req.TTL)
	}

	cred := &Credential{
		ID:        uuid.New().String(),
		Type:      "github_app",
		Token:     tokenResp.Token,
		ExpiresAt: expiresAt,
		Scope:     req.Target,
		TaskID:    req.TaskID,
		IssuedAt:  now,
	}

	log.Printf("[credential] issued GitHub App token for task %s (expires: %s)", req.TaskID, expiresAt.Format(time.RFC3339))
	return cred, nil
}

// Revoke marks a credential as revoked. GitHub installation tokens cannot be
// individually revoked via API, so we track it locally.
func (b *GitHubAppBroker) Revoke(_ context.Context, credID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revoked[credID] = true
	log.Printf("[credential] GitHub App credential %s revoked (local tracking only)", credID)
	return nil
}

// getJWT returns a JWT for authenticating as the GitHub App.
func (b *GitHubAppBroker) getJWT() (string, error) {
	if b.jwtFn != nil {
		return b.jwtFn()
	}
	// In production, this would read the private key from b.keyPath and
	// generate a signed JWT with the app ID. For now, return an error
	// indicating that a real key is needed.
	return "", fmt.Errorf("GitHub App JWT generation requires a private key at %s (not yet implemented — use WithJWTFunc for testing)", b.keyPath)
}
