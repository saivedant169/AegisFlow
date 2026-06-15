package gateway

import (
	"encoding/json"
	"strings"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// This file translates Anthropic Messages tool semantics to and from the
// internal (OpenAI-shaped) representation, so a Claude-native agentic
// conversation can be governed and proxied instead of rejected. It is only
// exercised when messages_api.tool_passthrough is enabled.

// anthropicTool is a tool definition in the Anthropic Messages request.
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// anthropicContentBlock is one block of an Anthropic message's content array. A
// block is a text block, a tool_use block (assistant requesting a tool), or a
// tool_result block (user returning a tool's output).
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// translateAnthropicTools maps Anthropic tool definitions to internal Tools.
func translateAnthropicTools(raw json.RawMessage) []types.Tool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var ats []anthropicTool
	if err := json.Unmarshal(raw, &ats); err != nil {
		return nil
	}
	tools := make([]types.Tool, 0, len(ats))
	for _, at := range ats {
		tools = append(tools, types.Tool{
			Type: "function",
			Function: types.ToolFunction{
				Name:        at.Name,
				Description: at.Description,
				Parameters:  at.InputSchema,
			},
		})
	}
	return tools
}

// translateAnthropicToolChoice maps Anthropic tool_choice to the OpenAI form the
// internal request uses. Anthropic: {"type":"auto"|"any"|"tool"|"none","name"?}.
// OpenAI: "auto"|"none"|"required" or {"type":"function","function":{"name"}}.
func translateAnthropicToolChoice(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return json.RawMessage(`"auto"`)
	case "none":
		return json.RawMessage(`"none"`)
	case "any":
		return json.RawMessage(`"required"`)
	case "tool":
		if tc.Name != "" {
			b, err := json.Marshal(map[string]any{
				"type":     "function",
				"function": map[string]string{"name": tc.Name},
			})
			if err == nil {
				return b
			}
		}
		return json.RawMessage(`"required"`)
	}
	return nil
}

// translateAnthropicMessage converts one Anthropic message into one or more
// internal messages. A plain-string message maps 1:1. A block array maps to: a
// single role message carrying the text + any tool_use blocks (as ToolCalls),
// followed by one role:"tool" message per tool_result block (OpenAI requires a
// distinct tool message per tool_call id).
func translateAnthropicMessage(m anthropicInMessage) []types.Message {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return []types.Message{{Role: m.Role, Content: s}}
	}

	var blocks []anthropicContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return []types.Message{{Role: m.Role, Content: flattenAnthropicContent(m.Content)}}
	}

	var textParts []string
	var toolCalls []types.ToolCall
	var toolResults []types.Message
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case "tool_use":
			toolCalls = append(toolCalls, types.ToolCall{
				ID:       b.ID,
				Type:     "function",
				Function: types.ToolCallFunction{Name: b.Name, Arguments: string(b.Input)},
			})
		case "tool_result":
			toolResults = append(toolResults, types.Message{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    flattenAnthropicContent(b.Content),
			})
		}
	}

	out := make([]types.Message, 0, 1+len(toolResults))
	if len(textParts) > 0 || len(toolCalls) > 0 {
		out = append(out, types.Message{
			Role:      m.Role,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		})
	}
	out = append(out, toolResults...)
	if len(out) == 0 {
		out = append(out, types.Message{Role: m.Role})
	}
	return out
}
