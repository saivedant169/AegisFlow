package credential

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockSTSClient implements STSClient for testing.
type mockSTSClient struct {
	// Response to return
	Creds *STSCredentials
	Err   error

	// Captured call arguments
	CalledRoleARN     string
	CalledSessionName string
	CalledDuration    time.Duration
	CalledExternalID  string
}

func (m *mockSTSClient) AssumeRole(_ context.Context, roleARN, sessionName string, duration time.Duration, externalID string) (*STSCredentials, error) {
	m.CalledRoleARN = roleARN
	m.CalledSessionName = sessionName
	m.CalledDuration = duration
	m.CalledExternalID = externalID
	return m.Creds, m.Err
}

func TestAWSSTSBrokerIssue(t *testing.T) {
	expiration := time.Now().Add(1 * time.Hour).UTC()
	mock := &mockSTSClient{
		Creds: &STSCredentials{
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			SessionToken:    "FwoGZXIvYXdzEBYaDHqa0AP",
			Expiration:      expiration,
		},
	}

	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN:           "arn:aws:iam::123456789012:role/AgentRole",
		Region:            "us-west-2",
		SessionNamePrefix: "aegisflow",
		DefaultTTL:        1 * time.Hour,
	}, mock)

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-abc-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred.Type != "aws_sts" {
		t.Errorf("type = %q, want %q", cred.Type, "aws_sts")
	}
	if cred.TaskID != "task-abc-123" {
		t.Errorf("task_id = %q, want %q", cred.TaskID, "task-abc-123")
	}
	if cred.Scope != "arn:aws:iam::123456789012:role/AgentRole" {
		t.Errorf("scope = %q, want role ARN", cred.Scope)
	}
	if !strings.Contains(cred.Token, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("token should contain access key ID")
	}
	if !strings.Contains(cred.Token, "FwoGZXIvYXdzEBYaDHqa0AP") {
		t.Errorf("token should contain session token")
	}
	if cred.ExpiresAt != expiration {
		t.Errorf("expires_at = %v, want %v", cred.ExpiresAt, expiration)
	}
	if cred.ID == "" {
		t.Error("credential ID should not be empty")
	}

	// Verify the mock was called with correct role ARN
	if mock.CalledRoleARN != "arn:aws:iam::123456789012:role/AgentRole" {
		t.Errorf("STS called with role = %q, want role ARN", mock.CalledRoleARN)
	}
}

func TestAWSSTSBrokerRevoke(t *testing.T) {
	mock := &mockSTSClient{}
	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN: "arn:aws:iam::123456789012:role/AgentRole",
	}, mock)

	err := broker.Revoke(context.Background(), "cred-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was recorded.
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if !broker.revoked["cred-123"] {
		t.Error("credential should be marked as revoked")
	}
}

func TestAWSSTSBrokerSessionName(t *testing.T) {
	mock := &mockSTSClient{
		Creds: &STSCredentials{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
			Expiration:      time.Now().Add(1 * time.Hour),
		},
	}

	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN:           "arn:aws:iam::123456789012:role/AgentRole",
		SessionNamePrefix: "aegisflow",
		DefaultTTL:        1 * time.Hour,
	}, mock)

	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-xyz-789",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(mock.CalledSessionName, "task-xyz-789") {
		t.Errorf("session name %q should contain task ID", mock.CalledSessionName)
	}
	if !strings.HasPrefix(mock.CalledSessionName, "aegisflow-") {
		t.Errorf("session name %q should start with prefix", mock.CalledSessionName)
	}
}

func TestAWSSTSBrokerTTL(t *testing.T) {
	mock := &mockSTSClient{
		Creds: &STSCredentials{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
			Expiration:      time.Now().Add(30 * time.Minute),
		},
	}

	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN:    "arn:aws:iam::123456789012:role/AgentRole",
		DefaultTTL: 1 * time.Hour,
	}, mock)

	// Test with explicit TTL in request
	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-ttl",
		TTL:    30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.CalledDuration != 30*time.Minute {
		t.Errorf("duration = %v, want 30m", mock.CalledDuration)
	}

	// Test default TTL
	_, err = broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-default-ttl",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.CalledDuration != 1*time.Hour {
		t.Errorf("duration = %v, want 1h (default)", mock.CalledDuration)
	}

	// Test TTL clamped to STS minimum (15 minutes)
	_, err = broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-short-ttl",
		TTL:    1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.CalledDuration != 15*time.Minute {
		t.Errorf("duration = %v, want 15m (STS minimum)", mock.CalledDuration)
	}
}

func TestAWSSTSBrokerError(t *testing.T) {
	mock := &mockSTSClient{
		Err: fmt.Errorf("AccessDenied: not authorized to perform sts:AssumeRole"),
	}

	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN: "arn:aws:iam::123456789012:role/AgentRole",
	}, mock)

	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-fail",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sts assume-role") {
		t.Errorf("error = %q, should contain 'sts assume-role'", err.Error())
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("error = %q, should contain original STS error", err.Error())
	}
}

func TestAWSSTSBrokerName(t *testing.T) {
	mock := &mockSTSClient{}
	broker := NewAWSSTSBroker("my-aws-broker", AWSSTSBrokerConfig{
		RoleARN: "arn:aws:iam::123456789012:role/AgentRole",
	}, mock)

	if broker.Name() != "my-aws-broker" {
		t.Errorf("Name() = %q, want %q", broker.Name(), "my-aws-broker")
	}
}

func TestAWSSTSBrokerExternalID(t *testing.T) {
	mock := &mockSTSClient{
		Creds: &STSCredentials{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
			Expiration:      time.Now().Add(1 * time.Hour),
		},
	}

	broker := NewAWSSTSBroker("aws-test", AWSSTSBrokerConfig{
		RoleARN:    "arn:aws:iam::123456789012:role/AgentRole",
		ExternalID: "ext-id-456",
	}, mock)

	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-ext",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.CalledExternalID != "ext-id-456" {
		t.Errorf("external ID = %q, want %q", mock.CalledExternalID, "ext-id-456")
	}
}

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"aegisflow-task-123", "aegisflow-task-123"},
		{"aegis flow!task#123", "aegisflowtask123"},
		{"a", "aegisflow-session"},                          // too short
		{strings.Repeat("a", 100), strings.Repeat("a", 64)}, // too long
	}
	for _, tt := range tests {
		got := sanitizeSessionName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeSessionName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseAssumeRoleResponse(t *testing.T) {
	xml := `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId>
      <SecretAccessKey>wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY</SecretAccessKey>
      <SessionToken>FwoGZXIvYXdzEBYaDHqa0AP</SessionToken>
      <Expiration>2025-01-01T12:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleResult>
</AssumeRoleResponse>`

	creds, err := parseAssumeRoleResponse([]byte(xml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("AccessKeyID = %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("SecretAccessKey = %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "FwoGZXIvYXdzEBYaDHqa0AP" {
		t.Errorf("SessionToken = %q", creds.SessionToken)
	}
	if creds.Expiration.Year() != 2025 {
		t.Errorf("Expiration year = %d, want 2025", creds.Expiration.Year())
	}
}
