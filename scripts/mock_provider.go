package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func main() {
	listenAddr := flag.String("listen", ":18080", "listen address")
	latency := flag.Duration("latency", 25*time.Millisecond, "response latency")
	flag.Parse()

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: newMockProviderHandler(*latency),
	}

	log.Printf("mock provider listening on %s with latency %s", *listenAddr, *latency)
	log.Fatal(server.ListenAndServe())
}

func newMockProviderHandler(latency time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if latency > 0 {
			time.Sleep(latency)
		}

		var req types.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		prompt := ""
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				prompt = msg.Content
			}
		}
		reply := "Benchmark response for: " + prompt
		resp := types.ChatCompletionResponse{
			ID:      "bench-mock-1",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []types.Choice{{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: reply},
				FinishReason: "stop",
			}},
			Usage: types.Usage{
				PromptTokens:     estimateTokens(prompt),
				CompletionTokens: estimateTokens(reply),
				TotalTokens:      estimateTokens(prompt) + estimateTokens(reply),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		models := types.ModelList{
			Object: "list",
			Data: []types.Model{
				{ID: "gpt-4o-mini", Object: "model", Provider: "bench-openai"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	return mux
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Fields(text)) * 2
}
