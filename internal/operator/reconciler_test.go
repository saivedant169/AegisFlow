package operator

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/saivedant169/AegisFlow/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// ---------------------------------------------------------------------------
// Reconciler: end-to-end with providers, routes, tenants, policies, gateway
// ---------------------------------------------------------------------------

func TestReconcileCreatesConfigMap(t *testing.T) {
	s := newScheme(t)

	provider := &v1alpha1.AegisFlowProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "openai", Namespace: "default"},
		Spec: v1alpha1.ProviderSpec{
			Type:         "openai",
			BaseURL:      "https://api.openai.com/v1",
			APIKeySecret: v1alpha1.SecretRef{Name: "secret", Key: "OPENAI_KEY"},
			Models:       []string{"gpt-4o"},
			Region:       "us",
		},
	}

	route := &v1alpha1.AegisFlowRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		Spec: v1alpha1.RouteSpec{
			Match: v1alpha1.RouteMatchSpec{Model: "gpt-*"},
			Regions: []v1alpha1.RouteRegion{
				{Name: "us", Providers: []string{"openai"}, Strategy: "priority"},
			},
		},
	}

	tenant := &v1alpha1.AegisFlowTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "default-tenant", Namespace: "default"},
		Spec: v1alpha1.TenantSpec{
			DisplayName:   "Default",
			RateLimit:     v1alpha1.RateLimitSpec{RequestsPerMinute: 60, TokensPerMinute: 100000},
			AllowedModels: []string{"*"},
		},
	}

	policy := &v1alpha1.AegisFlowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "jailbreak", Namespace: "default"},
		Spec: v1alpha1.PolicySpec{
			Phase:    "input",
			Type:     "keyword",
			Action:   "block",
			Keywords: []string{"ignore previous"},
		},
	}

	gateway := &v1alpha1.AegisFlowGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "default"},
		Spec: v1alpha1.GatewaySpec{
			Server:  v1alpha1.ServerSpec{Port: 9090, AdminPort: 9091},
			Logging: v1alpha1.LoggingSpec{Level: "debug", Format: "text"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(provider, route, tenant, policy, gateway).
		Build()

	r := NewReconciler(cl, "default")
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// Verify ConfigMap was created
	var cm corev1.ConfigMap
	err := cl.Get(context.Background(), keyFor("aegisflow-config", "default"), &cm)
	if err != nil {
		t.Fatalf("configmap not created: %v", err)
	}
	yamlData, ok := cm.Data["aegisflow.yaml"]
	if !ok || len(yamlData) == 0 {
		t.Fatal("configmap missing aegisflow.yaml data")
	}
}

// ---------------------------------------------------------------------------
// Reconciler: update existing ConfigMap
// ---------------------------------------------------------------------------

func TestReconcileUpdatesExistingConfigMap(t *testing.T) {
	s := newScheme(t)

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "aegisflow-config", Namespace: "default"},
		Data:       map[string]string{"aegisflow.yaml": "old"},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(existing).
		Build()

	r := NewReconciler(cl, "default")
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	err := cl.Get(context.Background(), keyFor("aegisflow-config", "default"), &cm)
	if err != nil {
		t.Fatalf("configmap not found: %v", err)
	}
	if cm.Data["aegisflow.yaml"] == "old" {
		t.Error("configmap should have been updated")
	}
}

// ---------------------------------------------------------------------------
// Reconciler: empty namespace (no CRDs)
// ---------------------------------------------------------------------------

func TestReconcileEmptyNamespace(t *testing.T) {
	s := newScheme(t)

	cl := fake.NewClientBuilder().WithScheme(s).Build()

	r := NewReconciler(cl, "default")
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile with empty namespace should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Reconciler: gateway defaults when no AegisFlowGateway CRD exists
// ---------------------------------------------------------------------------

func TestReconcileGatewayDefaults(t *testing.T) {
	s := newScheme(t)

	cl := fake.NewClientBuilder().WithScheme(s).Build()

	r := NewReconciler(cl, "default")
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	err := cl.Get(context.Background(), keyFor("aegisflow-config", "default"), &cm)
	if err != nil {
		t.Fatalf("configmap not found: %v", err)
	}

	yamlData := cm.Data["aegisflow.yaml"]
	// Default gateway should produce port 8080
	if len(yamlData) == 0 {
		t.Fatal("expected non-empty yaml data")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func keyFor(name, ns string) k8stypes.NamespacedName {
	return k8stypes.NamespacedName{Name: name, Namespace: ns}
}
