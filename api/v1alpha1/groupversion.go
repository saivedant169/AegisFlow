package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var SchemeGroupVersion = schema.GroupVersion{Group: "aegisflow.io", Version: "v1alpha1"}

var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&AegisFlowGateway{},
		&AegisFlowGatewayList{},
		&AegisFlowProvider{},
		&AegisFlowProviderList{},
		&AegisFlowRoute{},
		&AegisFlowRouteList{},
		&AegisFlowTenant{},
		&AegisFlowTenantList{},
		&AegisFlowPolicy{},
		&AegisFlowPolicyList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
