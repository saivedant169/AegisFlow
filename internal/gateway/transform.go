package gateway

import (
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
