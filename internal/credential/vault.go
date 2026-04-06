package credential

import (
	"bytes"
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

// VaultBroker issues short-lived database credentials via HashiCorp Vault's
// database secrets engine. It calls the Vault HTTP API directly using net/http.
type VaultBroker struct {
	name       string
	vaultAddr  string
	vaultToken string
	secretPath string
	defaultTTL time.Duration
	client     *http.Client

	mu     sync.Mutex
	leases map[string]string // credID -> lease_id
}

// NewVaultBroker creates a VaultBroker. Pass a non-nil httpClient to inject a
// test transport; if nil, http.DefaultClient is used.
func NewVaultBroker(name, vaultAddr, vaultToken, secretPath string, defaultTTL time.Duration, httpClient *http.Client) *VaultBroker {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &VaultBroker{
		name:       name,
		vaultAddr:  vaultAddr,
		vaultToken: vaultToken,
		secretPath: secretPath,
		defaultTTL: defaultTTL,
		client:     httpClient,
		leases:     make(map[string]string),
	}
}

// Name returns the broker name.
func (b *VaultBroker) Name() string { return b.name }

// vaultCredsResponse models the JSON returned by Vault's database creds endpoint.
type vaultCredsResponse struct {
	RequestID string `json:"request_id"`
	LeaseID   string `json:"lease_id"`
	Renewable bool   `json:"renewable"`
	LeaseDur  int    `json:"lease_duration"`
	Data      struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"data"`
	Errors []string `json:"errors"`
}

// Issue requests short-lived database credentials from Vault.
// The role name is taken from req.Target.
func (b *VaultBroker) Issue(ctx context.Context, req CredentialRequest) (*Credential, error) {
	roleName := req.Target
	if roleName == "" {
		return nil, fmt.Errorf("vault: credential request target (role name) is required")
	}

	url := fmt.Sprintf("%s/v1/%s/creds/%s", b.vaultAddr, b.secretPath, roleName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: build request: %w", err)
	}
	httpReq.Header.Set("X-Vault-Token", b.vaultToken)

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vault: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vault: read response: %w", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("vault: authentication error (403 Forbidden)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("vault: role %q not found (404)", roleName)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var vaultResp vaultCredsResponse
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, fmt.Errorf("vault: decode response: %w", err)
	}
	if len(vaultResp.Errors) > 0 {
		return nil, fmt.Errorf("vault: %s", vaultResp.Errors[0])
	}

	ttl := time.Duration(vaultResp.LeaseDur) * time.Second
	if req.TTL > 0 && req.TTL < ttl {
		ttl = req.TTL
	}
	if ttl == 0 {
		ttl = b.defaultTTL
	}

	now := time.Now().UTC()
	credID := uuid.New().String()

	b.mu.Lock()
	b.leases[credID] = vaultResp.LeaseID
	b.mu.Unlock()

	cred := &Credential{
		ID:        credID,
		Type:      "vault",
		Token:     vaultResp.Data.Username + ":" + vaultResp.Data.Password,
		ExpiresAt: now.Add(ttl),
		Scope:     req.Target,
		TaskID:    req.TaskID,
		IssuedAt:  now,
	}

	log.Printf("[credential] issued vault credential %s for task %s (role: %s, ttl: %s)", credID, req.TaskID, roleName, ttl)
	return cred, nil
}

// Revoke tells Vault to revoke the lease associated with the credential.
func (b *VaultBroker) Revoke(ctx context.Context, credID string) error {
	b.mu.Lock()
	leaseID, ok := b.leases[credID]
	if ok {
		delete(b.leases, credID)
	}
	b.mu.Unlock()

	if !ok {
		return fmt.Errorf("vault: no lease found for credential %s", credID)
	}

	url := fmt.Sprintf("%s/v1/sys/leases/revoke", b.vaultAddr)
	payload, _ := json.Marshal(map[string]string{"lease_id": leaseID})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("vault: build revoke request: %w", err)
	}
	httpReq.Header.Set("X-Vault-Token", b.vaultToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("vault: revoke request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault: revoke failed status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[credential] revoked vault credential %s (lease: %s)", credID, leaseID)
	return nil
}
