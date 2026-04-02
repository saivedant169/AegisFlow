package eval

import "github.com/saivedant169/AegisFlow/pkg/types"

// BuiltinResult holds the quality score and any detected issues for a response.
type BuiltinResult struct {
	Score  int      `json:"score"`
	Issues []string `json:"issues"`
}

// ScoreResponse evaluates a ChatCompletionResponse and returns a quality score (0-100).
// It checks for empty responses, truncation, short outputs, and latency degradation.
func ScoreResponse(resp *types.ChatCompletionResponse, latencyMs int64, avgP50Ms int64, minTokens int, latencyMultiplier float64) BuiltinResult {
	score := 100
	var issues []string

	// Empty response
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return BuiltinResult{Score: 0, Issues: []string{"empty response"}}
	}

	// Truncated
	if resp.Choices[0].FinishReason != "stop" && resp.Choices[0].FinishReason != "" {
		score -= 30
		issues = append(issues, "truncated response (finish_reason: "+resp.Choices[0].FinishReason+")")
	}

	// Too short
	if resp.Usage.CompletionTokens < minTokens && resp.Usage.PromptTokens > minTokens {
		score -= 20
		issues = append(issues, "response too short")
	}

	// Latency degradation
	if avgP50Ms > 0 && latencyMs > int64(float64(avgP50Ms)*latencyMultiplier) {
		score -= 10
		issues = append(issues, "latency degradation")
	}

	if score < 0 {
		score = 0
	}

	return BuiltinResult{Score: score, Issues: issues}
}
