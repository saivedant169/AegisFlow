package sqlgate

import (
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// Result holds the outcome of a SQL query evaluation.
type Result struct {
	Decision   envelope.Decision
	EnvelopeID string
	Message    string
	Operation  string
}

// Interceptor evaluates SQL queries against tool policies and records evidence.
type Interceptor struct {
	engine         *toolpolicy.Engine
	chain          *evidence.SessionChain
	blockDangerous bool
}

// NewInterceptor creates a new SQL query interceptor.
// If blockDangerous is true, queries classified as dangerous (DROP, TRUNCATE,
// DELETE/UPDATE without WHERE) are automatically blocked before policy evaluation.
func NewInterceptor(engine *toolpolicy.Engine, chain *evidence.SessionChain, blockDangerous bool) *Interceptor {
	return &Interceptor{
		engine:         engine,
		chain:          chain,
		blockDangerous: blockDangerous,
	}
}

// Evaluate parses the SQL query, checks it against tool policies, and records
// the action in the evidence chain. The database parameter is used as the
// envelope's Target field.
func (i *Interceptor) Evaluate(query string, database string) (*Result, error) {
	classification := ClassifySQL(query)

	// Map operation to capability.
	capability := operationCapability(classification.Operation)

	// Build tool name: "sql.{operation}".
	toolName := "sql." + classification.Operation

	// Create envelope.
	actor := envelope.ActorInfo{
		Type: "agent",
		ID:   "sql-interceptor",
	}
	env := envelope.NewEnvelope(actor, "sql-query", envelope.ProtocolSQL, toolName, database, capability)
	env.Parameters["query"] = query
	env.Parameters["table"] = classification.Table
	env.Parameters["has_where"] = classification.HasWhereClause
	env.Parameters["is_dangerous"] = classification.IsDangerous

	// If blockDangerous is enabled and query is dangerous, block immediately.
	if i.blockDangerous && classification.IsDangerous {
		env.PolicyDecision = envelope.DecisionBlock
		env.Justification = fmt.Sprintf("dangerous SQL operation: %s", classification.Operation)

		if _, err := i.chain.Record(env); err != nil {
			return nil, fmt.Errorf("recording evidence: %w", err)
		}

		return &Result{
			Decision:   envelope.DecisionBlock,
			EnvelopeID: env.ID,
			Message:    env.Justification,
			Operation:  classification.Operation,
		}, nil
	}

	// Evaluate against tool policy engine.
	decision := i.engine.Evaluate(env)
	env.PolicyDecision = decision

	// Build message.
	msg := fmt.Sprintf("sql %s on %s: %s", classification.Operation, database, decision)
	if classification.IsDangerous {
		msg = fmt.Sprintf("dangerous sql %s on %s: %s", classification.Operation, database, decision)
	}
	env.Justification = msg

	// Record in evidence chain.
	if _, err := i.chain.Record(env); err != nil {
		return nil, fmt.Errorf("recording evidence: %w", err)
	}

	return &Result{
		Decision:   decision,
		EnvelopeID: env.ID,
		Message:    msg,
		Operation:  classification.Operation,
	}, nil
}

// operationCapability maps SQL operation types to envelope capabilities.
func operationCapability(operation string) envelope.Capability {
	switch operation {
	case "select":
		return envelope.CapRead
	case "insert", "update", "create_table", "alter_table", "grant", "revoke":
		return envelope.CapWrite
	case "delete", "drop_table", "drop_database", "truncate":
		return envelope.CapDelete
	default:
		return envelope.CapExecute
	}
}
