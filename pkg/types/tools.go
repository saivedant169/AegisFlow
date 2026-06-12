package types

import "encoding/json"

// Tool is a function the model may call. AegisFlow's internal representation
// follows the OpenAI tool-calling shape; provider adapters translate to and
// from their own wire formats.
type Tool struct {
	Type     string       `json:"type"` // "function" (the only kind today)
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function and its JSON-Schema parameters.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
}

// ToolCall is a model's request to invoke a tool, carried on an assistant
// message.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction is the name and JSON-encoded arguments of a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded argument object
}

// ToolCallDelta is a partial tool call streamed across chunks. Index ties the
// fragments of one call together; Arguments arrives a piece at a time and must
// be concatenated by index.
type ToolCallDelta struct {
	Index    int                   `json:"index"`
	ID       string                `json:"id,omitempty"`
	Type     string                `json:"type,omitempty"`
	Function ToolCallFunctionDelta `json:"function,omitempty"`
}

// ToolCallFunctionDelta is the streamed fragment of a tool call's function.
type ToolCallFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
