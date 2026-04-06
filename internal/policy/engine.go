package policy

import (
	"fmt"
	"log"
	"strings"
)

type Action string

const (
	ActionBlock Action = "block"
	ActionWarn  Action = "warn"
	ActionLog   Action = "log"
)

// GovernanceMode controls how the policy engine handles filter errors.
type GovernanceMode string

const (
	// ModeGovernance is fail-closed: filter errors are treated as blocks.
	ModeGovernance GovernanceMode = "governance"
	// ModePermissive is fail-open: filter errors are logged and the request passes.
	ModePermissive GovernanceMode = "permissive"
)

type Violation struct {
	PolicyName string
	Action     Action
	Message    string
}

// Filter is the basic policy filter interface. Check returns a violation
// if the content violates the policy, or nil if it passes.
type Filter interface {
	Name() string
	Action() Action
	Check(content string) *Violation
}

// ErrorFilter is an optional interface that filters can implement when their
// check logic may return an error (e.g. WASM plugins, external calls).
// The engine will prefer CheckE over Check when available.
type ErrorFilter interface {
	Filter
	CheckE(content string) (*Violation, error)
}

type Engine struct {
	inputFilters  []Filter
	outputFilters []Filter
	mode          GovernanceMode
	breakGlass    bool
}

// NewEngine creates a policy engine with the given filters, governance mode,
// and break-glass override. When breakGlass is true, governance mode is
// overridden to permissive regardless of the mode parameter.
func NewEngine(inputFilters, outputFilters []Filter, opts ...EngineOption) *Engine {
	e := &Engine{
		inputFilters:  inputFilters,
		outputFilters: outputFilters,
		mode:          ModeGovernance,
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.breakGlass {
		log.Printf("WARNING: break-glass mode is active — policy engine running in permissive mode")
	}
	return e
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithGovernanceMode sets the governance mode for the engine.
func WithGovernanceMode(mode GovernanceMode) EngineOption {
	return func(e *Engine) {
		if mode == ModePermissive || mode == ModeGovernance {
			e.mode = mode
		}
	}
}

// WithBreakGlass enables break-glass mode, overriding governance to permissive.
func WithBreakGlass(enabled bool) EngineOption {
	return func(e *Engine) {
		e.breakGlass = enabled
	}
}

// effectiveMode returns the active governance mode, accounting for break-glass.
func (e *Engine) effectiveMode() GovernanceMode {
	if e.breakGlass {
		return ModePermissive
	}
	return e.mode
}

func (e *Engine) checkFilter(f Filter, content string) (*Violation, error) {
	if ef, ok := f.(ErrorFilter); ok {
		return ef.CheckE(content)
	}
	return f.Check(content), nil
}

func (e *Engine) handleFilterError(f Filter, err error) (*Violation, error) {
	mode := e.effectiveMode()
	if mode == ModeGovernance {
		return &Violation{
			PolicyName: f.Name(),
			Action:     ActionBlock,
			Message:    fmt.Sprintf("policy filter error (fail-closed): %v", err),
		}, nil
	}
	// Permissive: log and allow
	log.Printf("policy filter %q error (permissive mode, allowing): %v", f.Name(), err)
	return nil, nil
}

func (e *Engine) CheckInput(content string) (*Violation, error) {
	for _, f := range e.inputFilters {
		v, err := e.checkFilter(f, content)
		if err != nil {
			return e.handleFilterError(f, err)
		}
		if v != nil {
			return v, nil
		}
	}
	return nil, nil
}

func (e *Engine) CheckOutput(content string) (*Violation, error) {
	for _, f := range e.outputFilters {
		v, err := e.checkFilter(f, content)
		if err != nil {
			return e.handleFilterError(f, err)
		}
		if v != nil {
			return v, nil
		}
	}
	return nil, nil
}

func MessagesContent(messages []struct{ Role, Content string }) string {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return strings.Join(parts, " ")
}

func FormatViolation(v *Violation) string {
	return fmt.Sprintf("policy violation: %s — %s", v.PolicyName, v.Message)
}
