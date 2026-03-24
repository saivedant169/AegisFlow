package policy

import (
	"fmt"
	"strings"
)

type Action string

const (
	ActionBlock Action = "block"
	ActionWarn  Action = "warn"
	ActionLog   Action = "log"
)

type Violation struct {
	PolicyName string
	Action     Action
	Message    string
}

type Filter interface {
	Name() string
	Action() Action
	Check(content string) *Violation
}

type Engine struct {
	inputFilters  []Filter
	outputFilters []Filter
}

func NewEngine(inputFilters, outputFilters []Filter) *Engine {
	return &Engine{
		inputFilters:  inputFilters,
		outputFilters: outputFilters,
	}
}

func (e *Engine) CheckInput(content string) (*Violation, error) {
	for _, f := range e.inputFilters {
		if v := f.Check(content); v != nil {
			return v, nil
		}
	}
	return nil, nil
}

func (e *Engine) CheckOutput(content string) (*Violation, error) {
	for _, f := range e.outputFilters {
		if v := f.Check(content); v != nil {
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
