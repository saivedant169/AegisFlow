package operator

import (
	"context"
	"fmt"
	"log"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aegisflow/aegisflow/internal/config"
)

// Reconciler watches AegisFlow CRDs and generates a ConfigMap containing aegisflow.yaml.
type Reconciler struct {
	client    client.Client
	namespace string
}

// NewReconciler creates a new Reconciler that writes to the given namespace.
func NewReconciler(c client.Client, namespace string) *Reconciler {
	return &Reconciler{client: c, namespace: namespace}
}

// Reconcile reads all AegisFlow CRDs and generates a ConfigMap with aegisflow.yaml.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	cfg, err := r.buildConfig(ctx)
	if err != nil {
		return fmt.Errorf("building config: %w", err)
	}

	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Create or update ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aegisflow-config",
			Namespace: r.namespace,
		},
	}

	existing := &corev1.ConfigMap{}
	err = r.client.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	if err != nil {
		// Create
		cm.Data = map[string]string{"aegisflow.yaml": string(yamlBytes)}
		if createErr := r.client.Create(ctx, cm); createErr != nil {
			return fmt.Errorf("creating configmap: %w", createErr)
		}
		log.Printf("operator: created configmap aegisflow-config")
	} else {
		// Update
		existing.Data = map[string]string{"aegisflow.yaml": string(yamlBytes)}
		if updateErr := r.client.Update(ctx, existing); updateErr != nil {
			return fmt.Errorf("updating configmap: %w", updateErr)
		}
		log.Printf("operator: updated configmap aegisflow-config")
	}

	return nil
}

// buildConfig assembles a config.Config from structured inputs.
// In the full operator wiring (main.go), the controller lists CRD objects,
// converts them to Input types, and calls BuildConfig. For now this method
// provides the plumbing that connects BuildConfig output to the reconciler.
func (r *Reconciler) buildConfig(_ context.Context) (*config.Config, error) {
	// Stub: returns defaults. The operator main.go will populate these
	// by listing CRD resources and converting them to Input structs
	// before calling BuildConfig.
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081, LogLevel: "info", LogFormat: "json"},
		nil, nil, nil, nil,
	)
	return cfg, nil
}
