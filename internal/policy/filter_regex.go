package policy

import (
	"fmt"
	"regexp"
)

type RegexFilter struct {
	name     string
	action   Action
	patterns []*regexp.Regexp
}

func NewRegexFilter(name string, action Action, patterns []string) *RegexFilter {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, re)
		}
	}
	return &RegexFilter{
		name:     name,
		action:   action,
		patterns: compiled,
	}
}

func (f *RegexFilter) Name() string   { return f.name }
func (f *RegexFilter) Action() Action { return f.action }

func (f *RegexFilter) Check(content string) *Violation {
	for _, re := range f.patterns {
		if re.MatchString(content) {
			return &Violation{
				PolicyName: f.name,
				Action:     f.action,
				Message:    fmt.Sprintf("pattern matched: %s", re.String()),
			}
		}
	}
	return nil
}
