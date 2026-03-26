package provider

import (
	"time"
)

// Together AI is OpenAI-compatible. This is a thin wrapper around OpenAIProvider
// with Together-specific defaults.

func NewTogetherProvider(name, apiKeyEnv string, models []string, timeout time.Duration) *OpenAIProvider {
	if len(models) == 0 {
		models = []string{"meta-llama/Llama-3.3-70B-Instruct-Turbo", "mistralai/Mixtral-8x7B-Instruct-v0.1"}
	}
	return NewOpenAIProvider(name, "https://api.together.xyz/v1", apiKeyEnv, models, timeout, 2)
}
