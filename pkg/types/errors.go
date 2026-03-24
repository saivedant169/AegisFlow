package types

import "fmt"

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

func NewErrorResponse(code int, errType, message string) ErrorResponse {
	return ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Type:    errType,
		},
	}
}

var (
	ErrProviderUnavailable = &APIError{Code: 502, Type: "provider_error", Message: "all providers failed"}
	ErrRateLimited         = &APIError{Code: 429, Type: "rate_limit_error", Message: "rate limit exceeded"}
	ErrUnauthorized        = &APIError{Code: 401, Type: "authentication_error", Message: "invalid or missing API key"}
	ErrPolicyViolation     = &APIError{Code: 403, Type: "policy_violation", Message: "request blocked by policy"}
	ErrModelNotFound       = &APIError{Code: 404, Type: "not_found", Message: "no route found for requested model"}
)
