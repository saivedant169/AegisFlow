package identity

import (
	"fmt"
	"time"
)

// SeparationRule defines a separation-of-duties constraint that is evaluated
// before sensitive operations are permitted.
type SeparationRule struct {
	Name        string
	Description string
	Check       func(actor Identity, action string) error
}

// BreakGlassRecord is emitted when a break-glass action is detected and must
// be reviewed post-incident.
type BreakGlassRecord struct {
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason"`
}

// TemporaryException grants a time-bound privilege escalation that must carry
// an expiry.
type TemporaryException struct {
	GrantedTo string    `json:"granted_to"`
	Privilege string    `json:"privilege"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
}

// DefaultRules returns the built-in separation-of-duties rules.
func DefaultRules() []SeparationRule {
	return []SeparationRule{
		PolicyAuthorCannotApprove(),
		ConnectorAdminNotSessionOperator(),
		BreakGlassRequiresPostmortem(),
		ExceptionGrantsTimeBound(),
	}
}

// PolicyAuthorCannotApprove prevents a policy author from also approving
// policy changes in the same scope.
func PolicyAuthorCannotApprove() SeparationRule {
	return SeparationRule{
		Name:        "PolicyAuthorCannotApprove",
		Description: "A policy author cannot approve their own policy changes",
		Check: func(actor Identity, action string) error {
			if action != "approve_policy" {
				return nil
			}
			for _, role := range actor.Roles {
				if role.Name == "policy_author" {
					return fmt.Errorf("separation of duties: identity %q has role policy_author and cannot approve policies in scope %s/%s",
						actor.ID, role.Scope, role.ScopeID)
				}
			}
			return nil
		},
	}
}

// ConnectorAdminNotSessionOperator prevents a connector admin from also
// operating sessions on the same connector scope.
func ConnectorAdminNotSessionOperator() SeparationRule {
	return SeparationRule{
		Name:        "ConnectorAdminNotSessionOperator",
		Description: "A connector admin cannot also operate sessions on the same scope",
		Check: func(actor Identity, action string) error {
			if action != "operate_session" {
				return nil
			}
			for _, role := range actor.Roles {
				if role.Name == "admin" {
					return fmt.Errorf("separation of duties: identity %q has admin role and cannot operate sessions in scope %s/%s",
						actor.ID, role.Scope, role.ScopeID)
				}
			}
			return nil
		},
	}
}

// BreakGlassRequiresPostmortem flags any break-glass action for mandatory
// post-incident review.
func BreakGlassRequiresPostmortem() SeparationRule {
	return SeparationRule{
		Name:        "BreakGlassRequiresPostmortem",
		Description: "Break-glass actions are always flagged for post-incident review",
		Check: func(actor Identity, action string) error {
			if action == "break_glass" {
				return fmt.Errorf("break-glass by %q requires post-incident review", actor.ID)
			}
			return nil
		},
	}
}

// ExceptionGrantsTimeBound ensures that temporary privilege exceptions carry a
// non-zero expiry.
func ExceptionGrantsTimeBound() SeparationRule {
	return SeparationRule{
		Name:        "ExceptionGrantsTimeBound",
		Description: "Temporary exception grants must have an expiry time",
		Check: func(actor Identity, action string) error {
			if action == "grant_exception" {
				// When the action is grant_exception the caller must ensure
				// the TemporaryException has a non-zero ExpiresAt before
				// proceeding. This rule always returns an error as a
				// reminder that the caller must validate the expiry.
				return fmt.Errorf("exception grant by %q must include a valid expiry", actor.ID)
			}
			return nil
		},
	}
}

// ValidateException checks that a TemporaryException has a future expiry.
func ValidateException(exc TemporaryException) error {
	if exc.ExpiresAt.IsZero() {
		return fmt.Errorf("exception for %q must have an expiry time", exc.GrantedTo)
	}
	if exc.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("exception for %q has already expired", exc.GrantedTo)
	}
	return nil
}

// EvaluateRules runs all rules against the given actor and action, returning
// the first violation or nil.
func EvaluateRules(rules []SeparationRule, actor Identity, action string) error {
	for _, rule := range rules {
		if err := rule.Check(actor, action); err != nil {
			return err
		}
	}
	return nil
}
