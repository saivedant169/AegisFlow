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
