package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

// apiClient keeps credentials and error bodies inside the configured API boundary.
// Plugin downloads use their own unauthenticated clients.
type apiClient struct{ http *http.Client }

var client = &apiClient{http: &http.Client{
	Timeout:       10 * time.Second,
	Transport:     authenticatedTransport{},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}}

func apiBase(name, fallback string) (*url.URL, error) {
	u, err := url.Parse(getEnv(name, fallback))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("%s must be an HTTP(S) origin without credentials, path, query, or fragment", name)
	}
	return u, nil
}

type authenticatedTransport struct{}

func (authenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	admin, err := apiBase("AEGISFLOW_ADMIN_URL", defaultAdminURL)
	if err != nil {
		return nil, err
	}
	gateway, err := apiBase("AEGISFLOW_GATEWAY_URL", defaultGatewayURL)
	if err != nil {
		return nil, err
	}
	copy := req.Clone(req.Context())
	copy.Header.Del("X-API-Key")
	sameOrigin := func(base *url.URL) bool { return copy.URL.Scheme == base.Scheme && copy.URL.Host == base.Host }
	if (sameOrigin(admin) && strings.HasPrefix(copy.URL.Path, "/admin/v1/")) || (sameOrigin(gateway) && strings.HasPrefix(copy.URL.Path, "/v1/")) {
		if key := os.Getenv("AEGISFLOW_API_KEY"); key != "" {
			copy.Header.Set("X-API-Key", key)
		}
	}
	return http.DefaultTransport.RoundTrip(copy)
}

func (c *apiClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil || req.URL.User != nil || req.URL.Fragment != "" {
		return nil, errors.New("invalid API request URL")
	}
	// Validate separately so configuration errors name the setting without its value.
	for _, entry := range [][2]string{{"AEGISFLOW_ADMIN_URL", defaultAdminURL}, {"AEGISFLOW_GATEWAY_URL", defaultGatewayURL}} {
		if _, err := apiBase(entry[0], entry[1]); err != nil {
			return nil, err
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, errors.New("API request timed out")
		}
		return nil, errors.New("API request failed; check endpoint configuration and connectivity")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cleanup.Close(resp.Body)
		// Do not print response bodies: proxies can echo credentials and request data.
		return nil, fmt.Errorf("API returned HTTP %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return resp, nil
}

func (c *apiClient) Get(endpoint string) (*http.Response, error) {
	return c.request(http.MethodGet, endpoint, "", nil)
}
func (c *apiClient) Post(endpoint, contentType string, body io.Reader) (*http.Response, error) {
	return c.request(http.MethodPost, endpoint, contentType, body)
}
func (c *apiClient) request(method, endpoint, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, errors.New("invalid API request URL")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(req)
}
