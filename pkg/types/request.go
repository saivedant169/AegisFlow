package types

type ChatCompletionRequest struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	TopP        *float64          `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Metadata    map[string]string `json:"-"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
