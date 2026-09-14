package federation

import (
	"log"

	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/saivedant169/AegisFlow/internal/config"
)

const maxFederationBodySize = 1024 * 1024 // 1MB

// PlaneStatus tracks the health and metrics of a data plane.
type PlaneStatus struct {
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	LastSeen  time.Time `json:"last_seen"`
	Requests  int64     `json:"requests"`
	ErrorRate float64   `json:"error_rate"`
}

// ControlPlane manages data-plane registrations and serves stripped config.
type ControlPlane struct {
	cfg    *config.Config
	mu     sync.RWMutex
	planes map[string]*PlaneStatus
	tokens map[string]string // name -> token
}

// NewControlPlane creates a control plane from the gateway config.
func NewControlPlane(cfg *config.Config) *ControlPlane {
	cp := &ControlPlane{
		cfg:    cfg,
		planes: make(map[string]*PlaneStatus),
		tokens: make(map[string]string),
	}
	for _, dp := range cfg.Federation.DataPlanes {
		cp.planes[dp.Name] = &PlaneStatus{Name: dp.Name}
		cp.tokens[dp.Name] = dp.Token
	}
	return cp
}

// ConfigHandler serves the gateway config with sensitive fields stripped.
func (cp *ControlPlane) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if !cp.validateToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	stripped, err := cp.stripConfig()
	if err != nil {
		http.Error(w, "failed to prepare config", http.StatusInternalServerError)
		return
	}
	data, err := yaml.Marshal(stripped)
	if err != nil {
		http.Error(w, "failed to encode config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	_, _ = w.Write(data)
}

// MetricsHandler accepts metrics pushed from a data plane.
func (cp *ControlPlane) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	if !cp.validateToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"accepted"}`)); err != nil {
		return
	}
}

// StatusHandler accepts status updates from a data plane.
func (cp *ControlPlane) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if !cp.validateToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var status PlaneStatus
	if err := json.NewDecoder(io.LimitReader(r.Body, maxFederationBodySize)).Decode(&status); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cp.mu.Lock()
	if ps, ok := cp.planes[status.Name]; ok {
		ps.Healthy = status.Healthy
		ps.LastSeen = time.Now()
		ps.Requests = status.Requests
		ps.ErrorRate = status.ErrorRate
	}
	cp.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// PlanesHandler returns the list of known data planes and their status.
func (cp *ControlPlane) PlanesHandler(w http.ResponseWriter, _ *http.Request) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	planes := make([]PlaneStatus, 0, len(cp.planes))
	for _, ps := range cp.planes {
		planes = append(planes, *ps)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(planes); err != nil {
		log.Print("JSON response write failed")
		return
	}
}

func (cp *ControlPlane) validateToken(r *http.Request) bool {
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	if token == "" {
		return false
	}
	// Constant-time comparison to prevent timing attacks
	for _, t := range cp.tokens {
		if t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

func (cp *ControlPlane) stripConfig() (*config.Config, error) {
	data, err := yaml.Marshal(cp.cfg)
	if err != nil {
		return nil, err
	}

	stripped := &config.Config{}
	if err := yaml.Unmarshal(data, stripped); err != nil {
		return nil, err
	}

	for i := range stripped.Providers {
		for j := range stripped.Providers[i].APIKeys {
			stripped.Providers[i].APIKeys[j].Key = ""
		}
		for key := range stripped.Providers[i].Config {
			if sensitiveConfigKey(key) {
				delete(stripped.Providers[i].Config, key)
			}
		}
	}
	for i := range stripped.Tenants {
		stripped.Tenants[i].APIKeys = nil
	}
	for i := range stripped.Credentials.Providers {
		stripped.Credentials.Providers[i].Token = ""
		stripped.Credentials.Providers[i].GitHubKeyPath = ""
		stripped.Credentials.Providers[i].VaultToken = ""
		stripped.Credentials.Providers[i].AWSExternalID = ""
	}
	stripped.RateLimit.Redis.Password = ""
	stripped.Cache.Redis.Password = ""
	stripped.Cache.Semantic.APIKey = ""
	stripped.Webhook.Secret = ""
	stripped.Database.ConnString = ""
	stripped.Admin = config.AdminConfig{}
	stripped.Eval.Webhook.URL = ""
	stripped.ApprovalIntegrations.GitHub.Token = ""
	stripped.ApprovalIntegrations.Slack.WebhookURL = ""
	stripped.Capability.SigningKey = ""
	stripped.SupplyChain.SigningKey = ""
	stripped.Federation = config.FederationConfig{}
	return stripped, nil
}

func sensitiveConfigKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"credential", "key", "password", "secret", "token"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
