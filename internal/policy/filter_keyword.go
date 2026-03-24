package policy

import (
	"fmt"
	"strings"
)

type KeywordFilter struct {
	name     string
	action   Action
	keywords []string
}

func NewKeywordFilter(name string, action Action, keywords []string) *KeywordFilter {
	lower := make([]string, len(keywords))
	for i, k := range keywords {
		lower[i] = strings.ToLower(k)
	}
	return &KeywordFilter{
		name:     name,
		action:   action,
		keywords: lower,
	}
}

func (f *KeywordFilter) Name() string   { return f.name }
func (f *KeywordFilter) Action() Action { return f.action }

func (f *KeywordFilter) Check(content string) *Violation {
	lower := strings.ToLower(content)
	for _, kw := range f.keywords {
		if strings.Contains(lower, kw) {
			if f.action == ActionBlock {
				return &Violation{
					PolicyName: f.name,
					Action:     f.action,
					Message:    fmt.Sprintf("blocked keyword detected: %q", kw),
				}
			}
			return &Violation{
				PolicyName: f.name,
				Action:     f.action,
				Message:    fmt.Sprintf("keyword detected: %q", kw),
			}
		}
	}
	return nil
}
