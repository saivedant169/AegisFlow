package envelope

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/saivedant169/AegisFlow/internal/resource"
)

// ActionEnvelope normalizes every agent action into a policy-evaluable object.
type ActionEnvelope struct {
	ID                  string             `json:"id"`
	Timestamp           time.Time          `json:"timestamp"`
	Actor               ActorInfo          `json:"actor"`
	Task                string             `json:"task"`
	Protocol            Protocol           `json:"protocol"`
	Tool                string             `json:"tool"`
	Target              string             `json:"target"`
	Parameters          map[string]any     `json:"parameters"`
	RequestedCapability Capability         `json:"requested_capability"`
	Resource            *resource.Resource `json:"resource,omitempty"`
	CredentialRef       string             `json:"credential_ref,omitempty"`
	PolicyDecision      Decision           `json:"policy_decision"`
	EvidenceHash        string             `json:"evidence_hash,omitempty"`
	Justification       string             `json:"justification,omitempty"`
	Result              *ActionResult      `json:"result,omitempty"`
}

// ActorInfo identifies the entity performing the action.
type ActorInfo struct {
	Type      string `json:"type"` // "user", "agent", "service"
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	TenantID  string `json:"tenant_id"`
}

// Protocol represents the communication protocol of the action.
type Protocol string

const (
	ProtocolMCP   Protocol = "mcp"
	ProtocolHTTP  Protocol = "http"
	ProtocolShell Protocol = "shell"
	ProtocolSQL   Protocol = "sql"
	ProtocolGit   Protocol = "git"
)

// Capability represents the type of operation being requested.
type Capability string

const (
	CapRead    Capability = "read"
	CapWrite   Capability = "write"
	CapDelete  Capability = "delete"
	CapDeploy  Capability = "deploy"
	CapApprove Capability = "approve"
	CapExecute Capability = "execute"
)

// Decision represents the policy evaluation outcome.
type Decision string

const (
	DecisionPending Decision = "pending"
	DecisionAllow   Decision = "allow"
	DecisionReview  Decision = "review"
	DecisionBlock   Decision = "block"
)

// ActionResult captures the outcome of an executed action.
type ActionResult struct {
	Success    bool          `json:"success"`
	StatusCode int           `json:"status_code,omitempty"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// NewEnvelope creates a new ActionEnvelope with a generated UUID and current timestamp.
// The PolicyDecision is initialized to "pending".
func NewEnvelope(actor ActorInfo, task string, protocol Protocol, tool, target string, capability Capability) *ActionEnvelope {
	return &ActionEnvelope{
		ID:                  uuid.New().String(),
		Timestamp:           time.Now().UTC(),
		Actor:               actor,
		Task:                task,
		Protocol:            protocol,
		Tool:                tool,
		Target:              target,
		Parameters:          make(map[string]any),
		RequestedCapability: capability,
		PolicyDecision:      DecisionPending,
	}
}

// Hash computes a SHA-256 hash of the envelope content for the evidence chain.
// It hashes a deterministic JSON representation that excludes mutable fields
// (EvidenceHash and Result) to ensure stability.
func (e *ActionEnvelope) Hash() string {
	// Build a canonical representation excluding mutable fields.
	canonical := struct {
		ID                  string         `json:"id"`
		Timestamp           time.Time      `json:"timestamp"`
		Actor               ActorInfo      `json:"actor"`
		Task                string         `json:"task"`
		Protocol            Protocol       `json:"protocol"`
		Tool                string         `json:"tool"`
		Target              string         `json:"target"`
		Parameters          map[string]any `json:"parameters"`
		RequestedCapability Capability     `json:"requested_capability"`
		CredentialRef       string         `json:"credential_ref,omitempty"`
		PolicyDecision      Decision       `json:"policy_decision"`
		Justification       string         `json:"justification,omitempty"`
	}{
		ID:                  e.ID,
		Timestamp:           e.Timestamp,
		Actor:               e.Actor,
		Task:                e.Task,
		Protocol:            e.Protocol,
		Tool:                e.Tool,
		Target:              e.Target,
		Parameters:          e.Parameters,
		RequestedCapability: e.RequestedCapability,
		CredentialRef:       e.CredentialRef,
		PolicyDecision:      e.PolicyDecision,
		Justification:       e.Justification,
	}

	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// IsDestructive returns true if the requested capability is write, delete, or deploy.
func (e *ActionEnvelope) IsDestructive() bool {
	switch e.RequestedCapability {
	case CapWrite, CapDelete, CapDeploy:
		return true
	default:
		return false
	}
}

// Validate checks that all required fields are set on the envelope.
func (e *ActionEnvelope) Validate() error {
	var errs []error

	if e.ID == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if e.Timestamp.IsZero() {
		errs = append(errs, errors.New("timestamp is required"))
	}
	if e.Actor.Type == "" {
		errs = append(errs, errors.New("actor.type is required"))
	}
	if e.Actor.ID == "" {
		errs = append(errs, errors.New("actor.id is required"))
	}
	if e.Task == "" {
		errs = append(errs, errors.New("task is required"))
	}
	if e.Protocol == "" {
		errs = append(errs, errors.New("protocol is required"))
	}
	if e.Tool == "" {
		errs = append(errs, errors.New("tool is required"))
	}
	if e.Target == "" {
		errs = append(errs, errors.New("target is required"))
	}
	if e.RequestedCapability == "" {
		errs = append(errs, errors.New("requested_capability is required"))
	}

	return errors.Join(errs...)
}
