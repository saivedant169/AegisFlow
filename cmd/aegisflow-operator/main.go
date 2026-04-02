package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/saivedant169/AegisFlow/api/v1alpha1"
	"github.com/saivedant169/AegisFlow/internal/operator"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

type operatorReconciler struct {
	reconciler *operator.Reconciler
}

func (r *operatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if err := r.reconciler.Reconcile(ctx); err != nil {
		log.Printf("reconciliation error: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	return ctrl.Result{}, nil
}

func main() {
	namespace := flag.String("namespace", "default", "namespace to watch")
	metricsAddr := flag.String("metrics-bind-address", ":8090", "metrics endpoint")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: *metricsAddr,
		},
	})
	if err != nil {
		log.Fatalf("unable to create manager: %v", err)
		os.Exit(1)
	}

	reconciler := operator.NewReconciler(mgr.GetClient(), *namespace)

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AegisFlowGateway{}).
		Complete(&operatorReconciler{reconciler: reconciler}); err != nil {
		log.Fatalf("unable to create controller: %v", err)
		os.Exit(1)
	}

	log.Printf("aegisflow-operator starting (namespace: %s)", *namespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("unable to start manager: %v", err)
		os.Exit(1)
	}
}
