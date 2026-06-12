package types

import "encoding/json"

type ChatCompletionRequest struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	TopP        *float64          `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Tools       []Tool            `json:"tools,omitempty"`
	ToolChoice  json.RawMessage   `json:"tool_choice,omitempty"` // "auto"/"none"/"required" or a {type,function} object
	Metadata    map[string]string `json:"-"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls carries an assistant message's requests to invoke tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool-result message (role "tool") to the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}
