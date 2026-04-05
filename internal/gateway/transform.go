package gateway

import (
	"regexp"
	"strings"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// TransformConfig holds request transformation rules.
type TransformConfig struct {
	SystemPromptPrefix string            // Prepend to system message
	SystemPromptSuffix string            // Append to system message
	DefaultSystemPrompt string           // Use if no system message exists
	HeaderInjections    map[string]string // Extra metadata to inject
}

// TransformRequest applies transformation rules to the request before routing.
func TransformRequest(req *types.ChatCompletionRequest, cfg *TransformConfig) {
	if cfg == nil {
		return
	}

	hasSystem := false
	for i, msg := range req.Messages {
		if msg.Role == "system" {
			hasSystem = true
			if cfg.SystemPromptPrefix != "" {
				req.Messages[i].Content = cfg.SystemPromptPrefix + " " + msg.Content
			}
			if cfg.SystemPromptSuffix != "" {
				req.Messages[i].Content = msg.Content + " " + cfg.SystemPromptSuffix
			}
			break
		}
	}

	if !hasSystem && cfg.DefaultSystemPrompt != "" {
		req.Messages = append([]types.Message{
			{Role: "system", Content: cfg.DefaultSystemPrompt},
		}, req.Messages...)
	}
}

// TransformRequestWithTenant applies transforms with tenant overrides.
// Tenant config fields override global config fields when non-empty.
func TransformRequestWithTenant(req *types.ChatCompletionRequest, global, tenant *TransformConfig) {
	effective := mergeTransformConfig(global, tenant)
	TransformRequest(req, effective)
}

func mergeTransformConfig(global, tenant *TransformConfig) *TransformConfig {
	if tenant == nil {
		return global
	}
	if global == nil {
		return tenant
	}

	merged := &TransformConfig{}

	merged.SystemPromptPrefix = tenant.SystemPromptPrefix
	if merged.SystemPromptPrefix == "" {
		merged.SystemPromptPrefix = global.SystemPromptPrefix
	}

	merged.SystemPromptSuffix = tenant.SystemPromptSuffix
	if merged.SystemPromptSuffix == "" {
		merged.SystemPromptSuffix = global.SystemPromptSuffix
	}

	merged.DefaultSystemPrompt = tenant.DefaultSystemPrompt
	if merged.DefaultSystemPrompt == "" {
		merged.DefaultSystemPrompt = global.DefaultSystemPrompt
	}

	merged.HeaderInjections = make(map[string]string)
	for k, v := range global.HeaderInjections {
		merged.HeaderInjections[k] = v
	}
	for k, v := range tenant.HeaderInjections {
		merged.HeaderInjections[k] = v
	}

	return merged
}

// ResponseTransformConfig holds response transformation rules.
type ResponseTransformConfig struct {
	StripPII     bool              // Replace PII patterns with placeholders
	ContentSuffix string           // Append to response content
	ContentPrefix string           // Prepend to response content
	Replacements  map[string]string // Literal string replacements
}

var (
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex = regexp.MustCompile(`(\+?1[\s.-]?)?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}`)
	ssnRegex   = regexp.MustCompile(`\d{3}-\d{2}-\d{4}`)
	ccRegex    = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
)

// TransformResponse applies transformation rules to the response before returning to client.
func TransformResponse(resp *types.ChatCompletionResponse, cfg *ResponseTransformConfig) {
	if cfg == nil || len(resp.Choices) == 0 {
		return
	}

	for i := range resp.Choices {
		content := resp.Choices[i].Message.Content

		if cfg.StripPII {
			content = ssnRegex.ReplaceAllString(content, "[SSN]")
			content = ccRegex.ReplaceAllString(content, "[CREDIT_CARD]")
			content = emailRegex.ReplaceAllString(content, "[EMAIL]")
			content = phoneRegex.ReplaceAllString(content, "[PHONE]")
		}

		for old, replacement := range cfg.Replacements {
			content = strings.ReplaceAll(content, old, replacement)
		}

		if cfg.ContentPrefix != "" {
			content = cfg.ContentPrefix + content
		}
		if cfg.ContentSuffix != "" {
			content = content + cfg.ContentSuffix
		}

		resp.Choices[i].Message.Content = content
	}
}

// ApplyModelAlias rewrites the request model if it matches an alias.
func ApplyModelAlias(req *types.ChatCompletionRequest, aliases map[string]string) {
	if aliases == nil {
		return
	}
	if target, ok := aliases[req.Model]; ok {
		req.Model = target
	}
}
