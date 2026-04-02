package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestAzureOpenAIChatCompletion(t *testing.T) {
	mockResp := types.ChatCompletionResponse{
		ID: "azure-test", Object: "chat.completion", Model: "gpt-4o",
		Choices: []types.Choice{{Index: 0, Message: types.Message{Role: "assistant", Content: "Hello from Azure!"}, FinishReason: "stop"}},
		Usage:   types.Usage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Azure URL format
		if !strings.Contains(r.URL.Path, "/openai/deployments/") {
			t.Errorf("expected Azure deployment URL, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("api-version") == "" {
			t.Error("expected api-version query parameter")
		}
		// Verify Azure auth header
		if r.Header.Get("api-key") != "test-azure-key" {
			t.Errorf("expected api-key header, got %s", r.Header.Get("api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	p := &AzureOpenAIProvider{
		name: "azure-test", endpoint: srv.URL, apiKey: "test-azure-key",
		apiVersion: "2024-10-21", models: []string{"gpt-4o"}, client: srv.Client(),
	}

	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "gpt-4o", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello from Azure!" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
}

func TestAzureOpenAIBuildURL(t *testing.T) {
	p := &AzureOpenAIProvider{
		endpoint: "https://myresource.openai.azure.com", apiVersion: "2024-10-21",
	}
	url := p.buildURL("gpt-4o")
	expected := "https://myresource.openai.azure.com/openai/deployments/gpt-4o/chat/completions?api-version=2024-10-21"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestAzureOpenAIModels(t *testing.T) {
	p := &AzureOpenAIProvider{name: "azure", models: []string{"gpt-4o", "gpt-4o-mini"}}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if models[0].Provider != "azure" {
		t.Errorf("expected provider 'azure', got '%s'", models[0].Provider)
	}
}
