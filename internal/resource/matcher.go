package resource

import "path"

// Match checks whether a ResourceRule matches a given Resource and Verb.
// It supports exact match, glob on path segments, environment matching,
// and sensitivity thresholds.
func Match(rule ResourceRule, res Resource, verb Verb) bool {
	// Resource type match
	if rule.ResourceType != "" && rule.ResourceType != "*" {
		if ResourceType(rule.ResourceType) != res.Type {
			return false
		}
	}

	// Provider match
	if rule.Provider != "" && rule.Provider != "*" {
		if rule.Provider != res.Provider {
			return false
		}
	}

	// Environment match
	if rule.Environment != "" && rule.Environment != "*" {
		if rule.Environment != res.Environment {
			return false
		}
	}

	// Verb match
	if rule.Verb != "" && rule.Verb != "*" {
		if Verb(rule.Verb) != verb {
			return false
		}
	}

	// Sensitivity threshold: rule matches if resource sensitivity >= rule sensitivity
	if rule.Sensitivity != "" {
		ruleRank := sensitivityRank(Sensitivity(rule.Sensitivity))
		resRank := sensitivityRank(res.Sensitivity)
		if resRank < ruleRank {
			return false
		}
	}

	// Path pattern match (glob on joined path)
	if rule.PathPattern != "" && rule.PathPattern != "*" {
		resPath := res.PathString()
		matched, err := path.Match(rule.PathPattern, resPath)
		if err != nil || !matched {
			return false
		}
	}

	return true
}
