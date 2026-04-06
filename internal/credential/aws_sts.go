package credential

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// STSCredentials holds the temporary credentials returned by STS AssumeRole.
type STSCredentials struct {
	AccessKeyID     string
	SecretAccessKey  string
	SessionToken    string
	Expiration      time.Time
}

// STSClient abstracts the STS AssumeRole call for testability.
type STSClient interface {
	AssumeRole(ctx context.Context, roleARN, sessionName string, duration time.Duration, externalID string) (*STSCredentials, error)
}

// AWSSTSBrokerConfig holds configuration for the AWS STS broker.
type AWSSTSBrokerConfig struct {
	RoleARN           string
	Region            string
	SessionNamePrefix string
	ExternalID        string
	DefaultTTL        time.Duration
}

// AWSSTSBroker issues short-lived AWS credentials via STS AssumeRole.
type AWSSTSBroker struct {
	name   string
	config AWSSTSBrokerConfig
	client STSClient

	mu      sync.Mutex
	revoked map[string]bool
}

// NewAWSSTSBroker creates a new AWS STS credential broker.
func NewAWSSTSBroker(name string, cfg AWSSTSBrokerConfig, client STSClient) *AWSSTSBroker {
	if cfg.SessionNamePrefix == "" {
		cfg.SessionNamePrefix = "aegisflow"
	}
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 1 * time.Hour
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &AWSSTSBroker{
		name:    name,
		config:  cfg,
		client:  client,
		revoked: make(map[string]bool),
	}
}

// Name returns the broker name.
func (b *AWSSTSBroker) Name() string {
	return b.name
}

// Issue calls STS AssumeRole and returns a task-scoped credential.
func (b *AWSSTSBroker) Issue(ctx context.Context, req CredentialRequest) (*Credential, error) {
	ttl := b.config.DefaultTTL
	if req.TTL > 0 {
		ttl = req.TTL
	}

	// Clamp duration to STS limits: min 900s (15m), max 43200s (12h).
	if ttl < 15*time.Minute {
		ttl = 15 * time.Minute
	}
	if ttl > 12*time.Hour {
		ttl = 12 * time.Hour
	}

	sessionName := sanitizeSessionName(fmt.Sprintf("%s-%s", b.config.SessionNamePrefix, req.TaskID))

	stsCreds, err := b.client.AssumeRole(ctx, b.config.RoleARN, sessionName, ttl, b.config.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("sts assume-role: %w", err)
	}

	now := time.Now().UTC()
	// Encode all three fields so callers can extract them.
	token := fmt.Sprintf("%s:%s:%s", stsCreds.AccessKeyID, stsCreds.SecretAccessKey, stsCreds.SessionToken)

	cred := &Credential{
		ID:        uuid.New().String(),
		Type:      "aws_sts",
		Token:     token,
		ExpiresAt: stsCreds.Expiration,
		Scope:     b.config.RoleARN,
		TaskID:    req.TaskID,
		IssuedAt:  now,
	}

	log.Printf("[credential] issued AWS STS credential %s for task %s (role: %s, session: %s, expires: %s)",
		cred.ID, req.TaskID, b.config.RoleARN, sessionName, stsCreds.Expiration.Format(time.RFC3339))

	return cred, nil
}

// Revoke marks a credential as revoked. STS temporary credentials cannot be
// revoked server-side, but we record it locally and log the action.
func (b *AWSSTSBroker) Revoke(_ context.Context, credID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revoked[credID] = true
	log.Printf("[credential] AWS STS credential %s revoked (note: STS tokens cannot be invalidated server-side; rely on short TTL)", credID)
	return nil
}

// sanitizeSessionName ensures the session name conforms to AWS requirements:
// 2-64 characters, alphanumeric plus =,.@-_
func sanitizeSessionName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '=' || r == ',' || r == '.' || r == '@' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 64 {
		s = s[:64]
	}
	if len(s) < 2 {
		s = "aegisflow-session"
	}
	return s
}

// ---------------------------------------------------------------------------
// HTTPSTSClient: a real STS client using raw HTTP with AWS Signature V4.
// ---------------------------------------------------------------------------

// HTTPSTSClient calls the STS API directly via net/http with SigV4 signing.
type HTTPSTSClient struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	HTTPClient      *http.Client
}

// NewHTTPSTSClient creates a client that signs STS requests with the provided
// base credentials (typically long-lived IAM user or role credentials).
func NewHTTPSTSClient(accessKey, secretKey, region string) *HTTPSTSClient {
	return &HTTPSTSClient{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		Region:          region,
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// AssumeRole makes a signed POST to the STS AssumeRole API and parses the XML response.
func (c *HTTPSTSClient) AssumeRole(ctx context.Context, roleARN, sessionName string, duration time.Duration, externalID string) (*STSCredentials, error) {
	endpoint := fmt.Sprintf("https://sts.%s.amazonaws.com/", c.Region)

	params := url.Values{}
	params.Set("Action", "AssumeRole")
	params.Set("Version", "2011-06-15")
	params.Set("RoleArn", roleARN)
	params.Set("RoleSessionName", sessionName)
	params.Set("DurationSeconds", fmt.Sprintf("%d", int(duration.Seconds())))
	if externalID != "" {
		params.Set("ExternalId", externalID)
	}

	body := params.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Sign with AWS Signature V4
	now := time.Now().UTC()
	signV4(req, []byte(body), c.AccessKeyID, c.SecretAccessKey, c.Region, "sts", now)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sts request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading sts response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sts returned %d: %s", resp.StatusCode, string(respBody))
	}

	return parseAssumeRoleResponse(respBody)
}

// ---------------------------------------------------------------------------
// AWS Signature V4 implementation (minimal, STS-specific)
// ---------------------------------------------------------------------------

func signV4(req *http.Request, payload []byte, accessKey, secretKey, region, service string, t time.Time) {
	datestamp := t.Format("20060102")
	amzdate := t.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzdate)
	req.Header.Set("Host", req.URL.Host)

	// Step 1: canonical request
	payloadHash := sha256Hex(payload)
	signedHeaders := canonicalHeaders(req)
	canonicalReq := strings.Join([]string{
		req.Method,
		"/",
		"", // canonical query string (empty, we POST form body)
		formatCanonicalHeaders(req, signedHeaders),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	// Step 2: string to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzdate,
		credentialScope,
		sha256Hex([]byte(canonicalReq)),
	}, "\n")

	// Step 3: signing key
	signingKey := deriveSigningKey(secretKey, datestamp, region, service)

	// Step 4: signature
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Step 5: authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, strings.Join(signedHeaders, ";"), signature)
	req.Header.Set("Authorization", authHeader)
}

func canonicalHeaders(req *http.Request) []string {
	headers := []string{"content-type", "host", "x-amz-date"}
	sort.Strings(headers)
	return headers
}

func formatCanonicalHeaders(req *http.Request, signed []string) string {
	var b strings.Builder
	for _, h := range signed {
		var val string
		if h == "host" {
			val = req.URL.Host
		} else {
			val = req.Header.Get(http.CanonicalHeaderKey(h))
		}
		b.WriteString(h + ":" + strings.TrimSpace(val) + "\n")
	}
	return b.String()
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func deriveSigningKey(secretKey, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// ---------------------------------------------------------------------------
// XML response parsing for STS AssumeRole
// ---------------------------------------------------------------------------

type assumeRoleResponse struct {
	XMLName xml.Name `xml:"AssumeRoleResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleResult"`
}

func parseAssumeRoleResponse(data []byte) (*STSCredentials, error) {
	var resp assumeRoleResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing sts xml: %w", err)
	}

	expiration, err := time.Parse(time.RFC3339, resp.Result.Credentials.Expiration)
	if err != nil {
		return nil, fmt.Errorf("parsing expiration: %w", err)
	}

	return &STSCredentials{
		AccessKeyID:     resp.Result.Credentials.AccessKeyId,
		SecretAccessKey:  resp.Result.Credentials.SecretAccessKey,
		SessionToken:    resp.Result.Credentials.SessionToken,
		Expiration:      expiration,
	}, nil
}
