package provider

import (
	"time"
)

// Groq is OpenAI-compatible. This is a thin wrapper around OpenAIProvider
// with Groq-specific defaults.

func NewGroqProvider(name, apiKeyEnv string, models []string, timeout time.Duration) *OpenAIProvider {
	if len(models) == 0 {
		models = []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "mixtral-8x7b-32768"}
	}
	return NewOpenAIProvider(name, "https://api.groq.com/openai/v1", apiKeyEnv, models, timeout, 2)
}
