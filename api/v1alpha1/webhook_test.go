package v1alpha1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------- AegisFlowGateway ----------

func TestGatewayValidator_ValidCreate(t *testing.T) {
	v := &GatewayValidator{}
	gw := &AegisFlowGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: GatewaySpec{
			Server:  ServerSpec{Port: 8080, AdminPort: 8081},
			Logging: LoggingSpec{Level: "info"},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), gw); err != nil {
		t.Fatalf("expected valid gateway to pass, got: %v", err)
	}
}

func TestGatewayValidator_InvalidPort(t *testing.T) {
	v := &GatewayValidator{}
	tests := []struct {
		name      string
		port      int
		adminPort int
	}{
		{"port too high", 70000, 8081},
		{"port negative", -1, 8081},
		{"admin port too high", 8080, 100000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &AegisFlowGateway{
				Spec: GatewaySpec{
					Server: ServerSpec{Port: tt.port, AdminPort: tt.adminPort},
				},
			}
			if _, err := v.ValidateCreate(context.Background(), gw); err == nil {
				t.Fatal("expected error for invalid port")
			}
		})
	}
}

func TestGatewayValidator_InvalidLogLevel(t *testing.T) {
	v := &GatewayValidator{}
	gw := &AegisFlowGateway{
		Spec: GatewaySpec{
			Server:  ServerSpec{Port: 8080},
			Logging: LoggingSpec{Level: "verbose"},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), gw); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestGatewayValidator_ZeroPortsAllowed(t *testing.T) {
	v := &GatewayValidator{}
	gw := &AegisFlowGateway{
		Spec: GatewaySpec{
			Server: ServerSpec{Port: 0, AdminPort: 0},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), gw); err != nil {
		t.Fatalf("zero ports (omitted) should be allowed, got: %v", err)
	}
}

func TestGatewayValidator_Update(t *testing.T) {
	v := &GatewayValidator{}
	old := &AegisFlowGateway{Spec: GatewaySpec{Server: ServerSpec{Port: 8080}}}
	bad := &AegisFlowGateway{Spec: GatewaySpec{Server: ServerSpec{Port: 99999}}}
	if _, err := v.ValidateUpdate(context.Background(), old, bad); err == nil {
		t.Fatal("expected error on update with invalid port")
	}
}

func TestGatewayValidator_DeleteAlwaysAllowed(t *testing.T) {
	v := &GatewayValidator{}
	if _, err := v.ValidateDelete(context.Background(), &AegisFlowGateway{}); err != nil {
		t.Fatalf("delete should always be allowed, got: %v", err)
	}
}

// ---------- AegisFlowProvider ----------

func TestProviderValidator_ValidCreate(t *testing.T) {
	v := &ProviderValidator{}
	p := &AegisFlowProvider{
		Spec: ProviderSpec{
			Type:    "openai",
			BaseURL: "https://api.openai.com/v1",
		},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err != nil {
		t.Fatalf("expected valid provider to pass, got: %v", err)
	}
}

func TestProviderValidator_UnknownType(t *testing.T) {
	v := &ProviderValidator{}
	p := &AegisFlowProvider{
		Spec: ProviderSpec{Type: "unknown-provider"},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for unknown provider type")
	}
}

func TestProviderValidator_EmptyType(t *testing.T) {
	v := &ProviderValidator{}
	p := &AegisFlowProvider{
		Spec: ProviderSpec{Type: ""},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for empty provider type")
	}
}

func TestProviderValidator_InvalidURL(t *testing.T) {
	v := &ProviderValidator{}
	p := &AegisFlowProvider{
		Spec: ProviderSpec{Type: "openai", BaseURL: "not a url"},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestProviderValidator_AllValidTypes(t *testing.T) {
	v := &ProviderValidator{}
	for pt := range validProviderTypes {
		t.Run(pt, func(t *testing.T) {
			p := &AegisFlowProvider{Spec: ProviderSpec{Type: pt}}
			if _, err := v.ValidateCreate(context.Background(), p); err != nil {
				t.Fatalf("type %q should be valid, got: %v", pt, err)
			}
		})
	}
}

// ---------- AegisFlowRoute ----------

func TestRouteValidator_ValidCreate(t *testing.T) {
	v := &RouteValidator{}
	r := &AegisFlowRoute{
		Spec: RouteSpec{
			Match: RouteMatchSpec{Model: "gpt-4*"},
			Regions: []RouteRegion{
				{Name: "us-east", Providers: []string{"openai-east"}},
			},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), r); err != nil {
		t.Fatalf("expected valid route to pass, got: %v", err)
	}
}

func TestRouteValidator_EmptyModelPattern(t *testing.T) {
	v := &RouteValidator{}
	r := &AegisFlowRoute{
		Spec: RouteSpec{
			Match: RouteMatchSpec{Model: ""},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), r); err == nil {
		t.Fatal("expected error for empty model pattern")
	}
}

func TestRouteValidator_EmptyProviders(t *testing.T) {
	v := &RouteValidator{}
	r := &AegisFlowRoute{
		Spec: RouteSpec{
			Match: RouteMatchSpec{Model: "gpt-4*"},
			Regions: []RouteRegion{
				{Name: "us-east", Providers: []string{}},
			},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), r); err == nil {
		t.Fatal("expected error for empty providers in region")
	}
}

// ---------- AegisFlowTenant ----------

func TestTenantValidator_ValidCreate(t *testing.T) {
	v := &TenantValidator{}
	tenant := &AegisFlowTenant{
		Spec: TenantSpec{
			APIKeySecrets: []SecretRef{{Name: "key-secret", Key: "api-key"}},
			RateLimit:     RateLimitSpec{RequestsPerMinute: 100, TokensPerMinute: 50000},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), tenant); err != nil {
		t.Fatalf("expected valid tenant to pass, got: %v", err)
	}
}

func TestTenantValidator_NegativeRateLimit(t *testing.T) {
	v := &TenantValidator{}
	tenant := &AegisFlowTenant{
		Spec: TenantSpec{
			APIKeySecrets: []SecretRef{{Name: "key-secret", Key: "api-key"}},
			RateLimit:     RateLimitSpec{RequestsPerMinute: -5},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), tenant); err == nil {
		t.Fatal("expected error for negative rate limit")
	}
}

func TestTenantValidator_EmptyAPIKeySecrets(t *testing.T) {
	v := &TenantValidator{}
	tenant := &AegisFlowTenant{
		Spec: TenantSpec{
			APIKeySecrets: nil,
		},
	}
	if _, err := v.ValidateCreate(context.Background(), tenant); err == nil {
		t.Fatal("expected error for empty API key secrets")
	}
}

func TestTenantValidator_EmptySecretName(t *testing.T) {
	v := &TenantValidator{}
	tenant := &AegisFlowTenant{
		Spec: TenantSpec{
			APIKeySecrets: []SecretRef{{Name: "", Key: "api-key"}},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), tenant); err == nil {
		t.Fatal("expected error for empty secret name")
	}
}

// ---------- AegisFlowPolicy ----------

func TestPolicyValidator_ValidCreate(t *testing.T) {
	v := &PolicyValidator{}
	p := &AegisFlowPolicy{
		Spec: PolicySpec{Type: "keyword", Action: "block", Keywords: []string{"secret"}},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err != nil {
		t.Fatalf("expected valid policy to pass, got: %v", err)
	}
}

func TestPolicyValidator_InvalidType(t *testing.T) {
	v := &PolicyValidator{}
	p := &AegisFlowPolicy{
		Spec: PolicySpec{Type: "unknown", Action: "block"},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for invalid policy type")
	}
}

func TestPolicyValidator_InvalidAction(t *testing.T) {
	v := &PolicyValidator{}
	p := &AegisFlowPolicy{
		Spec: PolicySpec{Type: "keyword", Action: "delete"},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for invalid policy action")
	}
}

func TestPolicyValidator_EmptyType(t *testing.T) {
	v := &PolicyValidator{}
	p := &AegisFlowPolicy{
		Spec: PolicySpec{Type: "", Action: "block"},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for empty policy type")
	}
}

func TestPolicyValidator_EmptyAction(t *testing.T) {
	v := &PolicyValidator{}
	p := &AegisFlowPolicy{
		Spec: PolicySpec{Type: "keyword", Action: ""},
	}
	if _, err := v.ValidateCreate(context.Background(), p); err == nil {
		t.Fatal("expected error for empty policy action")
	}
}

func TestPolicyValidator_AllValidTypes(t *testing.T) {
	v := &PolicyValidator{}
	for pt := range validPolicyTypes {
		t.Run(pt, func(t *testing.T) {
			p := &AegisFlowPolicy{Spec: PolicySpec{Type: pt, Action: "block"}}
			if _, err := v.ValidateCreate(context.Background(), p); err != nil {
				t.Fatalf("policy type %q should be valid, got: %v", pt, err)
			}
		})
	}
}

func TestPolicyValidator_AllValidActions(t *testing.T) {
	v := &PolicyValidator{}
	for action := range validPolicyActions {
		t.Run(action, func(t *testing.T) {
			p := &AegisFlowPolicy{Spec: PolicySpec{Type: "keyword", Action: action}}
			if _, err := v.ValidateCreate(context.Background(), p); err != nil {
				t.Fatalf("policy action %q should be valid, got: %v", action, err)
			}
		})
	}
}
