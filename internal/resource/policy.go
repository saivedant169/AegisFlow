package resource

import "sort"

// Decision represents the policy evaluation outcome for resource rules.
type Decision string

const (
	DecisionAllow   Decision = "allow"
	DecisionReview  Decision = "review"
	DecisionBlock   Decision = "block"
	DecisionPending Decision = "pending"
)

// ResourceRule defines a single resource-level policy rule.
type ResourceRule struct {
	ResourceType string `yaml:"resource_type" json:"resource_type"` // e.g. "repo", "table", "*"
	Provider     string `yaml:"provider" json:"provider"`           // e.g. "github", "postgres", "*"
	Environment  string `yaml:"environment" json:"environment"`     // e.g. "prod", "staging", "*"
	PathPattern  string `yaml:"path_pattern" json:"path_pattern"`   // glob, e.g. "myorg/*/main"
	Sensitivity  string `yaml:"sensitivity" json:"sensitivity"`     // threshold: matches resources at or above
	Verb         string `yaml:"verb" json:"verb"`                   // e.g. "delete", "read", "*"
	Decision     string `yaml:"decision" json:"decision"`           // "allow", "review", "block"
	Priority     int    `yaml:"priority" json:"priority"`           // lower number = higher priority
}

// ResourcePolicyEngine evaluates Resources against typed resource rules.
type ResourcePolicyEngine struct {
	rules           []ResourceRule
	defaultDecision Decision
}

// NewResourcePolicyEngine creates an engine with sorted rules and a default decision.
func NewResourcePolicyEngine(rules []ResourceRule, defaultDecision string) *ResourcePolicyEngine {
	dd := Decision(defaultDecision)
	switch dd {
	case DecisionAllow, DecisionReview, DecisionBlock:
		// valid
	default:
		dd = DecisionBlock
	}

	// Sort rules by priority (ascending = higher priority first).
	sorted := make([]ResourceRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	// Expand environment inheritance: prod rules generate staging rules
	// at priority+1000 if no explicit staging rule exists.
	expanded := expandEnvironmentInheritance(sorted)

	return &ResourcePolicyEngine{
		rules:           expanded,
		defaultDecision: dd,
	}
}

// expandEnvironmentInheritance creates staging rules from prod rules unless
// a staging rule already exists for the same resource type/provider/path/verb.
func expandEnvironmentInheritance(rules []ResourceRule) []ResourceRule {
	type ruleKey struct {
		resourceType string
		provider     string
		pathPattern  string
		verb         string
	}

	stagingKeys := make(map[ruleKey]bool)
	for _, r := range rules {
		if r.Environment == "staging" {
			stagingKeys[ruleKey{r.ResourceType, r.Provider, r.PathPattern, r.Verb}] = true
		}
	}

	var inherited []ResourceRule
	for _, r := range rules {
		if r.Environment == "prod" {
			key := ruleKey{r.ResourceType, r.Provider, r.PathPattern, r.Verb}
			if !stagingKeys[key] {
				staging := r
				staging.Environment = "staging"
				staging.Priority = r.Priority + 1000
				inherited = append(inherited, staging)
			}
		}
	}

	result := make([]ResourceRule, 0, len(rules)+len(inherited))
	result = append(result, rules...)
	result = append(result, inherited...)

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Priority < result[j].Priority
	})

	return result
}

// Evaluate checks a Resource and Verb against the rules.
// Deny-overrides: if any matching rule is "block", the result is block
// regardless of allow rules. Otherwise, highest-priority matching rule wins.
func (e *ResourcePolicyEngine) Evaluate(res Resource, verb Verb) Decision {
	var matched []ResourceRule
	for _, rule := range e.rules {
		if Match(rule, res, verb) {
			matched = append(matched, rule)
		}
	}

	if len(matched) == 0 {
		return e.defaultDecision
	}

	// Deny-overrides: any block rule wins.
	for _, m := range matched {
		if Decision(m.Decision) == DecisionBlock {
			return DecisionBlock
		}
	}

	// Otherwise return highest-priority (first) matched rule's decision.
	return Decision(matched[0].Decision)
}
