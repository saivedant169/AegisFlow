package provider

import (
	"time"
)

// Mistral AI is OpenAI-compatible. This is a thin wrapper around OpenAIProvider
// with Mistral-specific defaults.

func NewMistralProvider(name, apiKeyEnv string, models []string, timeout time.Duration) *OpenAIProvider {
	if len(models) == 0 {
		models = []string{"mistral-large-latest", "mistral-small-latest", "open-mistral-nemo"}
	}
	return NewOpenAIProvider(name, "https://api.mistral.ai/v1", apiKeyEnv, models, timeout, 2)
}
