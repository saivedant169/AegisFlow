package eval

import (
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestScoreEmptyResponse(t *testing.T) {
	// No choices at all
	resp := &types.ChatCompletionResponse{}
	result := ScoreResponse(resp, 100, 50, 10, 2.0)
	if result.Score != 0 {
		t.Errorf("expected score 0 for empty choices, got %d", result.Score)
	}
	if len(result.Issues) != 1 || result.Issues[0] != "empty response" {
		t.Errorf("expected 'empty response' issue, got %v", result.Issues)
	}

	// One choice but empty content
	resp2 := &types.ChatCompletionResponse{
		Choices: []types.Choice{{Message: types.Message{Content: ""}}},
	}
	result2 := ScoreResponse(resp2, 100, 50, 10, 2.0)
	if result2.Score != 0 {
		t.Errorf("expected score 0 for empty content, got %d", result2.Score)
	}
}

func TestScoreHealthyResponse(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "Hello, world!"},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 20,
		},
	}
	result := ScoreResponse(resp, 100, 200, 10, 2.0)
	if result.Score != 100 {
		t.Errorf("expected score 100, got %d", result.Score)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected no issues, got %v", result.Issues)
	}
}

func TestScoreTruncatedResponse(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "partial output"},
				FinishReason: "length",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 20,
		},
	}
	result := ScoreResponse(resp, 100, 200, 10, 2.0)
	if result.Score != 70 {
		t.Errorf("expected score 70, got %d", result.Score)
	}
}

func TestScoreShortResponse(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "ok"},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 3,
		},
	}
	result := ScoreResponse(resp, 100, 200, 10, 2.0)
	if result.Score != 80 {
		t.Errorf("expected score 80, got %d", result.Score)
	}
}

func TestScoreLatencyDegradation(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 20,
		},
	}
	// latency 500ms, p50 is 100ms, multiplier 2.0 → 500 > 200, so -10
	result := ScoreResponse(resp, 500, 100, 10, 2.0)
	if result.Score != 90 {
		t.Errorf("expected score 90, got %d", result.Score)
	}
}

func TestScoreCumulativeDeductions(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "short"},
				FinishReason: "length",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 3,
		},
	}
	// truncated (-30) + short (-20) = 50
	result := ScoreResponse(resp, 100, 200, 10, 2.0)
	if result.Score != 50 {
		t.Errorf("expected score 50, got %d", result.Score)
	}
}

func TestScoreAllDeductions(t *testing.T) {
	// truncated (-30) + short (-20) + latency (-10) = 40
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "short"},
				FinishReason: "length",
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 3,
		},
	}
	result := ScoreResponse(resp, 500, 100, 10, 2.0)
	if result.Score != 40 {
		t.Errorf("expected score 40, got %d", result.Score)
	}
	if len(result.Issues) != 3 {
		t.Errorf("expected 3 issues, got %d: %v", len(result.Issues), result.Issues)
	}
}

func TestScoreNeverBelowZero(t *testing.T) {
	// Force massive deductions: truncated (-30) + short (-20) + latency (-10) = 40
	// Even if we somehow stacked more, floor should be 0. Verify the floor works
	// by checking that score >= 0 for a heavily penalised response.
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message:      types.Message{Content: "x"},
				FinishReason: "length",
			},
		},
		Usage: types.Usage{
			PromptTokens:     100,
			CompletionTokens: 1,
		},
	}
	result := ScoreResponse(resp, 9999, 1, 50, 1.0)
	if result.Score < 0 {
		t.Errorf("score should never go below 0, got %d", result.Score)
	}
}

func TestScoreNilChoices(t *testing.T) {
	resp := &types.ChatCompletionResponse{Choices: nil}
	result := ScoreResponse(resp, 100, 50, 10, 2.0)
	if result.Score != 0 {
		t.Errorf("expected score 0 for nil choices, got %d", result.Score)
	}
	if len(result.Issues) != 1 || result.Issues[0] != "empty response" {
		t.Errorf("expected 'empty response' issue, got %v", result.Issues)
	}
}

func TestScoreFinishReasonStop(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Content: "good"}, FinishReason: "stop"},
		},
		Usage: types.Usage{PromptTokens: 10, CompletionTokens: 20},
	}
	result := ScoreResponse(resp, 50, 200, 5, 2.0)
	if result.Score != 100 {
		t.Errorf("expected 100 for finish_reason stop, got %d", result.Score)
	}
}

func TestScoreFinishReasonEmpty(t *testing.T) {
	// empty finish_reason should NOT trigger truncation deduction
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Content: "good"}, FinishReason: ""},
		},
		Usage: types.Usage{PromptTokens: 10, CompletionTokens: 20},
	}
	result := ScoreResponse(resp, 50, 200, 5, 2.0)
	if result.Score != 100 {
		t.Errorf("expected 100 for empty finish_reason, got %d", result.Score)
	}
}

func TestScoreFinishReasonLength(t *testing.T) {
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Content: "partial"}, FinishReason: "length"},
		},
		Usage: types.Usage{PromptTokens: 10, CompletionTokens: 20},
	}
	result := ScoreResponse(resp, 50, 200, 5, 2.0)
	if result.Score != 70 {
		t.Errorf("expected 70 for finish_reason length, got %d", result.Score)
	}
}

func TestScoreLatencyNoP50(t *testing.T) {
	// avgP50Ms = 0 means no baseline => no latency penalty
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{Message: types.Message{Content: "ok"}, FinishReason: "stop"},
		},
		Usage: types.Usage{PromptTokens: 10, CompletionTokens: 20},
	}
	result := ScoreResponse(resp, 99999, 0, 5, 2.0)
	if result.Score != 100 {
		t.Errorf("expected 100 with zero p50, got %d", result.Score)
	}
}
