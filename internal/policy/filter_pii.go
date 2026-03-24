package policy

import (
	"fmt"
	"regexp"
)

var piiPatterns = map[string]*regexp.Regexp{
	"email":       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	"ssn":         regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	"credit_card": regexp.MustCompile(`\b(?:\d{4}[- ]?){3}\d{4}\b`),
	"phone":       regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),
}

type PIIFilter struct {
	name     string
	action   Action
	patterns map[string]*regexp.Regexp
}

func NewPIIFilter(name string, action Action, types []string) *PIIFilter {
	selected := make(map[string]*regexp.Regexp)
	for _, t := range types {
		if re, ok := piiPatterns[t]; ok {
			selected[t] = re
		}
	}
	if len(selected) == 0 {
		selected = piiPatterns
	}
	return &PIIFilter{
		name:     name,
		action:   action,
		patterns: selected,
	}
}

func (f *PIIFilter) Name() string   { return f.name }
func (f *PIIFilter) Action() Action { return f.action }

func (f *PIIFilter) Check(content string) *Violation {
	for piiType, re := range f.patterns {
		if re.MatchString(content) {
			return &Violation{
				PolicyName: f.name,
				Action:     f.action,
				Message:    fmt.Sprintf("PII detected: %s", piiType),
			}
		}
	}
	return nil
}
