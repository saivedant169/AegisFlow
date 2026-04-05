package gateway

import (
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestTransformResponseStripEmail(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: "Contact us at user@example.com for help."}},
		},
	}

	cfg := &ResponseTransformConfig{
		StripPII: true,
	}
	TransformResponse(resp, cfg)

	content := resp.Choices[0].Message.Content
	if content == "Contact us at user@example.com for help." {
		t.Fatal("expected email to be stripped")
	}
	if !strings.Contains(content, "[EMAIL]") {
		t.Fatalf("expected [EMAIL] placeholder, got: %s", content)
	}
}

func TestTransformResponseStripPhone(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: "Call me at 555-123-4567 or (555) 987-6543."}},
		},
	}

	cfg := &ResponseTransformConfig{
		StripPII: true,
	}
	TransformResponse(resp, cfg)

	content := resp.Choices[0].Message.Content
	if strings.Contains(content, "555-123-4567") || strings.Contains(content, "(555) 987-6543") {
		t.Fatalf("expected phone numbers to be stripped, got: %s", content)
	}
}

func TestTransformResponseStripSSN(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: "SSN is 123-45-6789."}},
		},
	}

	cfg := &ResponseTransformConfig{
		StripPII: true,
	}
	TransformResponse(resp, cfg)

	content := resp.Choices[0].Message.Content
	if strings.Contains(content, "123-45-6789") {
		t.Fatalf("expected SSN to be stripped, got: %s", content)
	}
}

func TestTransformResponseNoStripWhenDisabled(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: "Contact user@example.com"}},
		},
	}

	cfg := &ResponseTransformConfig{
		StripPII: false,
	}
	TransformResponse(resp, cfg)

	if resp.Choices[0].Message.Content != "Contact user@example.com" {
		t.Fatal("should not strip when disabled")
	}
}

func TestTransformResponseNilConfig(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: "hello"}},
		},
	}
	TransformResponse(resp, nil) // should not panic
	if resp.Choices[0].Message.Content != "hello" {
		t.Fatal("nil config should be no-op")
	}
}

func TestTransformRequestPerTenant(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "hello"},
		},
	}

	global := &TransformConfig{
		DefaultSystemPrompt: "You are a helpful assistant.",
	}
	tenant := &TransformConfig{
		DefaultSystemPrompt: "You are a customer support agent for Acme Corp.",
	}

	TransformRequestWithTenant(req, global, tenant)

	// Tenant config should override global
	if req.Messages[0].Role != "system" {
		t.Fatal("expected system message to be injected")
	}
	if req.Messages[0].Content != "You are a customer support agent for Acme Corp." {
		t.Fatalf("expected tenant override, got: %s", req.Messages[0].Content)
	}
}

func TestTransformRequestPerTenantFallsBackToGlobal(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "hello"},
		},
	}

	global := &TransformConfig{
		DefaultSystemPrompt: "You are a helpful assistant.",
	}

	TransformRequestWithTenant(req, global, nil)

	if req.Messages[0].Content != "You are a helpful assistant." {
		t.Fatalf("expected global fallback, got: %s", req.Messages[0].Content)
	}
}

func TestTransformRequestPerTenantMergesPrefix(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "system", Content: "base instructions"},
			{Role: "user", Content: "hello"},
		},
	}

	global := &TransformConfig{
		SystemPromptPrefix: "GLOBAL:",
	}
	tenant := &TransformConfig{
		SystemPromptPrefix: "TENANT:",
	}

	TransformRequestWithTenant(req, global, tenant)

	// Tenant prefix should win
	if req.Messages[0].Content != "TENANT: base instructions" {
		t.Fatalf("expected tenant prefix, got: %s", req.Messages[0].Content)
	}
}

func TestTransformRequestModelAlias(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "our-smart-model",
		Messages: []types.Message{
			{Role: "user", Content: "hello"},
		},
	}

	aliases := map[string]string{
		"our-smart-model": "gpt-4o",
		"our-fast-model":  "gpt-4o-mini",
	}

	ApplyModelAlias(req, aliases)

	if req.Model != "gpt-4o" {
		t.Fatalf("expected model to be aliased to gpt-4o, got: %s", req.Model)
	}
}

func TestTransformRequestModelAliasNoMatch(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "hello"},
		},
	}

	aliases := map[string]string{
		"our-smart-model": "gpt-4o",
	}

	ApplyModelAlias(req, aliases)

	if req.Model != "gpt-4o" {
		t.Fatalf("model should remain unchanged, got: %s", req.Model)
	}
}

func TestTransformRequestModelAliasNilMap(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
	}
	ApplyModelAlias(req, nil) // should not panic
	if req.Model != "gpt-4o" {
		t.Fatal("nil aliases should be no-op")
	}
}
