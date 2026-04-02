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

	v1alpha1 "github.com/saivedant169/AegisFlow/api/v1alpha1"
	"github.com/saivedant169/AegisFlow/internal/config"
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

// buildConfig lists all AegisFlow CRD resources and assembles them into a config.Config.
func (r *Reconciler) buildConfig(ctx context.Context) (*config.Config, error) {
	// List providers
	var providerList v1alpha1.AegisFlowProviderList
	if err := r.client.List(ctx, &providerList, client.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}
	var providers []ProviderInput
	for _, p := range providerList.Items {
		providers = append(providers, ProviderInput{
			Name:      p.Name,
			Type:      p.Spec.Type,
			BaseURL:   p.Spec.BaseURL,
			APIKeyEnv: p.Spec.APIKeySecret.Key,
			Models:    p.Spec.Models,
			Region:    p.Spec.Region,
		})
	}

	// List routes
	var routeList v1alpha1.AegisFlowRouteList
	if err := r.client.List(ctx, &routeList, client.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}
	var routes []RouteInput
	for _, rt := range routeList.Items {
		ri := RouteInput{Model: rt.Spec.Match.Model}
		for _, reg := range rt.Spec.Regions {
			ri.Regions = append(ri.Regions, RegionInput{
				Name:      reg.Name,
				Providers: reg.Providers,
				Strategy:  reg.Strategy,
			})
		}
		routes = append(routes, ri)
	}

	// List tenants
	var tenantList v1alpha1.AegisFlowTenantList
	if err := r.client.List(ctx, &tenantList, client.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	var tenants []TenantInput
	for _, t := range tenantList.Items {
		tenants = append(tenants, TenantInput{
			ID:                t.Name,
			Name:              t.Spec.DisplayName,
			RequestsPerMinute: t.Spec.RateLimit.RequestsPerMinute,
			TokensPerMinute:   t.Spec.RateLimit.TokensPerMinute,
			AllowedModels:     t.Spec.AllowedModels,
		})
	}

	// List policies
	var policyList v1alpha1.AegisFlowPolicyList
	if err := r.client.List(ctx, &policyList, client.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}
	var policies []PolicyInput
	for _, p := range policyList.Items {
		policies = append(policies, PolicyInput{
			Name:     p.Name,
			Phase:    p.Spec.Phase,
			Type:     p.Spec.Type,
			Action:   p.Spec.Action,
			Keywords: p.Spec.Keywords,
			Patterns: p.Spec.Patterns,
			WasmPath: p.Spec.WasmPath,
			OnError:  p.Spec.OnError,
		})
	}

	// Read gateway config
	gw := GatewayInput{Port: 8080, AdminPort: 8081, LogLevel: "info", LogFormat: "json"}
	var gwList v1alpha1.AegisFlowGatewayList
	if err := r.client.List(ctx, &gwList, client.InNamespace(r.namespace)); err == nil && len(gwList.Items) > 0 {
		g := gwList.Items[0]
		if g.Spec.Server.Port > 0 {
			gw.Port = g.Spec.Server.Port
		}
		if g.Spec.Server.AdminPort > 0 {
			gw.AdminPort = g.Spec.Server.AdminPort
		}
		if g.Spec.Logging.Level != "" {
			gw.LogLevel = g.Spec.Logging.Level
		}
		if g.Spec.Logging.Format != "" {
			gw.LogFormat = g.Spec.Logging.Format
		}
	}

	return BuildConfig(gw, providers, routes, tenants, policies), nil
}
