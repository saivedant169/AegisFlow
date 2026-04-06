package credential

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	return b.signJWT()
}

// signJWT reads the PEM private key from disk and produces an RS256 JWT
// suitable for GitHub App authentication.
func (b *GitHubAppBroker) signJWT() (string, error) {
	keyData, err := os.ReadFile(b.keyPath)
	if err != nil {
		return "", fmt.Errorf("reading private key file %s: %w", b.keyPath, err)
	}

	key, err := parseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return "", fmt.Errorf("parsing private key: %w", err)
	}

	now := time.Now()
	return buildRS256JWT(key, b.appID, now.Add(-60*time.Second), now.Add(10*time.Minute))
}

// parseRSAPrivateKeyFromPEM decodes and parses an RSA private key from PEM bytes.
// Supports both PKCS#1 (RSA PRIVATE KEY) and PKCS#8 (PRIVATE KEY) formats.
func parseRSAPrivateKeyFromPEM(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in key data")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}

// buildRS256JWT constructs and signs a minimal JWT with RS256 (RFC 7518).
// The token contains iss (app ID), iat, and exp claims.
func buildRS256JWT(key *rsa.PrivateKey, appID int64, iat, exp time.Time) (string, error) {
	header := base64URLEncode([]byte(`{"alg":"RS256","typ":"JWT"}`))

	payload, err := json.Marshal(map[string]interface{}{
		"iss": fmt.Sprintf("%d", appID),
		"iat": iat.Unix(),
		"exp": exp.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshalling JWT claims: %w", err)
	}
	encodedPayload := base64URLEncode(payload)

	signingInput := header + "." + encodedPayload

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	return signingInput + "." + base64URLEncode(sig), nil
}

// base64URLEncode performs base64url encoding without padding (per RFC 7515).
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// VerifyRS256JWT is exported for testing: it verifies an RS256 JWT signature
// and returns the decoded claims. Not intended for production use.
func VerifyRS256JWT(token string, pub *rsa.PublicKey) (map[string]interface{}, error) {
	parts := splitJWT(token)
	if parts == nil {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts")
	}

	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decoding signature: %w", err)
	}

	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sig); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("parsing claims: %w", err)
	}
	return claims, nil
}

// splitJWT splits a JWT into its three dot-separated parts, or returns nil.
func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	if len(parts) != 3 {
		return nil
	}
	return parts
}

