package gateway

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var (
	chatCompletionSchema *jsonschema.Schema
	completionSchema     *jsonschema.Schema
	embeddingSchema      *jsonschema.Schema
)

func init() {
	chatCompletionSchema = jsonschema.MustCompileString("chat_completion.json", `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"model": { "type": "string", "minLength": 1 },
			"messages": {
				"type": "array",
				"items": { "type": "object" },
				"minItems": 1
			}
		},
		"required": ["model", "messages"],
		"additionalProperties": true
	}`)

	completionSchema = jsonschema.MustCompileString("completion.json", `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"model": { "type": "string", "minLength": 1 },
			"prompt": {
				"anyOf": [
					{ "type": "string" },
					{ "type": "array", "items": { "type": "string" } }
				]
			}
		},
		"required": ["model", "prompt"],
		"additionalProperties": true
	}`)

	embeddingSchema = jsonschema.MustCompileString("embedding.json", `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"model": { "type": "string", "minLength": 1 },
			"input": {
				"anyOf": [
					{ "type": "string" },
					{ "type": "array", "items": { "type": "string" } }
				]
			}
		},
		"required": ["model", "input"],
		"additionalProperties": true
	}`)
}

// formatValidationError maps a jsonschema.ValidationError to an OpenAI-compatible error (message, param, code).
func formatValidationError(err error) (message, param, code string) {
	validationErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error(), "", "invalid_request_error"
	}

	if len(validationErr.Causes) > 0 {
		// Use the first cause for more specific error message
		cause := validationErr.Causes[0]
		param = strings.TrimPrefix(cause.InstanceLocation, "/")
		param = strings.ReplaceAll(param, "/", ".")
		message = cause.Message

		if strings.Contains(cause.Message, "missing properties") {
			parts := strings.Split(cause.Message, "'")
			if len(parts) >= 3 {
				param = parts[1]
				message = fmt.Sprintf("Missing required parameter: '%s'.", param)
			}
		} else if strings.Contains(cause.Message, "expected") {
			code = "invalid_type"
		}
	} else {
		param = strings.TrimPrefix(validationErr.InstanceLocation, "/")
		param = strings.ReplaceAll(param, "/", ".")
		message = validationErr.Message

		if strings.Contains(validationErr.Message, "missing properties") {
			parts := strings.Split(validationErr.Message, "'")
			if len(parts) >= 3 {
				param = parts[1]
				message = fmt.Sprintf("Missing required parameter: '%s'.", param)
			}
		} else if strings.Contains(validationErr.Message, "expected") {
			code = "invalid_type"
		}
	}

	if code == "" {
		code = "invalid_request_error"
	}

	return message, param, code
}

// validateRequest is a helper that parses the JSON body into a map and validates it against the schema.
func validateRequest(body []byte, schema *jsonschema.Schema) (message, param, code string, err error) {
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return "failed to parse request body", "", "invalid_request_error", err
	}

	if err := schema.Validate(v); err != nil {
		message, param, code = formatValidationError(err)
		return message, param, code, err
	}

	return "", "", "", nil
}
