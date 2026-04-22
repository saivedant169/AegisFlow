package v1alpha1

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ---------- AegisFlowGateway ----------

var validLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "error": true,
}

type GatewayValidator struct{}

var _ admission.Validator[*AegisFlowGateway] = &GatewayValidator{}

func (v *GatewayValidator) ValidateCreate(_ context.Context, obj *AegisFlowGateway) (admission.Warnings, error) {
	return validateGateway(obj)
}

func (v *GatewayValidator) ValidateUpdate(_ context.Context, _, newObj *AegisFlowGateway) (admission.Warnings, error) {
	return validateGateway(newObj)
}

func (v *GatewayValidator) ValidateDelete(_ context.Context, _ *AegisFlowGateway) (admission.Warnings, error) {
	return nil, nil
}

func validateGateway(gw *AegisFlowGateway) (admission.Warnings, error) {
	var errs []string

	if gw.Spec.Server.Port != 0 && (gw.Spec.Server.Port < 1 || gw.Spec.Server.Port > 65535) {
		errs = append(errs, fmt.Sprintf("spec.server.port must be between 1 and 65535, got %d", gw.Spec.Server.Port))
	}
	if gw.Spec.Server.AdminPort != 0 && (gw.Spec.Server.AdminPort < 1 || gw.Spec.Server.AdminPort > 65535) {
		errs = append(errs, fmt.Sprintf("spec.server.adminPort must be between 1 and 65535, got %d", gw.Spec.Server.AdminPort))
	}
	if gw.Spec.Logging.Level != "" && !validLogLevels[gw.Spec.Logging.Level] {
		errs = append(errs, fmt.Sprintf("spec.logging.level must be one of [debug, info, warn, error], got %q", gw.Spec.Logging.Level))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}
	return nil, nil
}

// ---------- AegisFlowProvider ----------

var validProviderTypes = map[string]bool{
	"openai": true, "anthropic": true, "ollama": true, "gemini": true,
	"azure": true, "groq": true, "mistral": true, "together": true, "bedrock": true,
}

type ProviderValidator struct{}

var _ admission.Validator[*AegisFlowProvider] = &ProviderValidator{}

func (v *ProviderValidator) ValidateCreate(_ context.Context, obj *AegisFlowProvider) (admission.Warnings, error) {
	return validateProvider(obj)
}

func (v *ProviderValidator) ValidateUpdate(_ context.Context, _, newObj *AegisFlowProvider) (admission.Warnings, error) {
	return validateProvider(newObj)
}

func (v *ProviderValidator) ValidateDelete(_ context.Context, _ *AegisFlowProvider) (admission.Warnings, error) {
	return nil, nil
}

func validateProvider(p *AegisFlowProvider) (admission.Warnings, error) {
	var errs []string

	if p.Spec.Type == "" {
		errs = append(errs, "spec.type is required")
	} else if !validProviderTypes[p.Spec.Type] {
		errs = append(errs, fmt.Sprintf("spec.type must be one of [openai, anthropic, ollama, gemini, azure, groq, mistral, together, bedrock], got %q", p.Spec.Type))
	}

	if p.Spec.BaseURL != "" {
		if _, err := url.ParseRequestURI(p.Spec.BaseURL); err != nil {
			errs = append(errs, fmt.Sprintf("spec.baseURL is not a valid URL: %v", err))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}
	return nil, nil
}

// ---------- AegisFlowRoute ----------

type RouteValidator struct{}

var _ admission.Validator[*AegisFlowRoute] = &RouteValidator{}

func (v *RouteValidator) ValidateCreate(_ context.Context, obj *AegisFlowRoute) (admission.Warnings, error) {
	return validateRoute(obj)
}

func (v *RouteValidator) ValidateUpdate(_ context.Context, _, newObj *AegisFlowRoute) (admission.Warnings, error) {
	return validateRoute(newObj)
}

func (v *RouteValidator) ValidateDelete(_ context.Context, _ *AegisFlowRoute) (admission.Warnings, error) {
	return nil, nil
}

func validateRoute(r *AegisFlowRoute) (admission.Warnings, error) {
	var errs []string

	if r.Spec.Match.Model == "" {
		errs = append(errs, "spec.match.model must be non-empty")
	}

	for i, region := range r.Spec.Regions {
		if len(region.Providers) == 0 {
			errs = append(errs, fmt.Sprintf("spec.regions[%d].providers must not be empty", i))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}
	return nil, nil
}

// ---------- AegisFlowTenant ----------

type TenantValidator struct{}

var _ admission.Validator[*AegisFlowTenant] = &TenantValidator{}

func (v *TenantValidator) ValidateCreate(_ context.Context, obj *AegisFlowTenant) (admission.Warnings, error) {
	return validateTenant(obj)
}

func (v *TenantValidator) ValidateUpdate(_ context.Context, _, newObj *AegisFlowTenant) (admission.Warnings, error) {
	return validateTenant(newObj)
}

func (v *TenantValidator) ValidateDelete(_ context.Context, _ *AegisFlowTenant) (admission.Warnings, error) {
	return nil, nil
}

func validateTenant(t *AegisFlowTenant) (admission.Warnings, error) {
	var errs []string

	if t.Spec.RateLimit.RequestsPerMinute < 0 {
		errs = append(errs, fmt.Sprintf("spec.rateLimit.requestsPerMinute must be non-negative, got %d", t.Spec.RateLimit.RequestsPerMinute))
	}
	if t.Spec.RateLimit.TokensPerMinute < 0 {
		errs = append(errs, fmt.Sprintf("spec.rateLimit.tokensPerMinute must be non-negative, got %d", t.Spec.RateLimit.TokensPerMinute))
	}
	if len(t.Spec.APIKeySecrets) == 0 {
		errs = append(errs, "spec.apiKeySecrets must not be empty")
	}
	for i, ref := range t.Spec.APIKeySecrets {
		if ref.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.apiKeySecrets[%d].name must not be empty", i))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}
	return nil, nil
}

// ---------- AegisFlowPolicy ----------

var validPolicyTypes = map[string]bool{
	"keyword": true, "regex": true, "pii": true, "wasm": true,
}

var validPolicyActions = map[string]bool{
	"block": true, "warn": true,
}

type PolicyValidator struct{}

var _ admission.Validator[*AegisFlowPolicy] = &PolicyValidator{}

func (v *PolicyValidator) ValidateCreate(_ context.Context, obj *AegisFlowPolicy) (admission.Warnings, error) {
	return validatePolicy(obj)
}

func (v *PolicyValidator) ValidateUpdate(_ context.Context, _, newObj *AegisFlowPolicy) (admission.Warnings, error) {
	return validatePolicy(newObj)
}

func (v *PolicyValidator) ValidateDelete(_ context.Context, _ *AegisFlowPolicy) (admission.Warnings, error) {
	return nil, nil
}

func validatePolicy(p *AegisFlowPolicy) (admission.Warnings, error) {
	var errs []string

	if p.Spec.Type == "" {
		errs = append(errs, "spec.type is required")
	} else if !validPolicyTypes[p.Spec.Type] {
		errs = append(errs, fmt.Sprintf("spec.type must be one of [keyword, regex, pii, wasm], got %q", p.Spec.Type))
	}

	if p.Spec.Action == "" {
		errs = append(errs, "spec.action is required")
	} else if !validPolicyActions[p.Spec.Action] {
		errs = append(errs, fmt.Sprintf("spec.action must be one of [block, warn], got %q", p.Spec.Action))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}
	return nil, nil
}

// SetupWebhooksWithManager registers all validating webhooks with the manager.
func SetupWebhooksWithManager(scheme *runtime.Scheme, webhookServer WebhookRegistrar) {
	webhookServer.Register("/validate-aegisflow-io-v1alpha1-aegisflowgateway",
		admission.WithValidator(scheme, &GatewayValidator{}))
	webhookServer.Register("/validate-aegisflow-io-v1alpha1-aegisflowprovider",
		admission.WithValidator(scheme, &ProviderValidator{}))
	webhookServer.Register("/validate-aegisflow-io-v1alpha1-aegisflowroute",
		admission.WithValidator(scheme, &RouteValidator{}))
	webhookServer.Register("/validate-aegisflow-io-v1alpha1-aegisflowtenant",
		admission.WithValidator(scheme, &TenantValidator{}))
	webhookServer.Register("/validate-aegisflow-io-v1alpha1-aegisflowpolicy",
		admission.WithValidator(scheme, &PolicyValidator{}))
}

// WebhookRegistrar is implemented by webhook.Server from controller-runtime.
type WebhookRegistrar interface {
	Register(path string, hook http.Handler)
}
