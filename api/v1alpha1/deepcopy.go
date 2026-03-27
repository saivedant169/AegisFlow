package v1alpha1

import "k8s.io/apimachinery/pkg/runtime"

// --- AegisFlowGateway ---

func (in *AegisFlowGateway) DeepCopyInto(out *AegisFlowGateway) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *AegisFlowGateway) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowGateway)
	in.DeepCopyInto(out)
	return out
}

func (in *AegisFlowGatewayList) DeepCopyInto(out *AegisFlowGatewayList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AegisFlowGateway, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AegisFlowGatewayList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowGatewayList)
	in.DeepCopyInto(out)
	return out
}

// --- AegisFlowProvider ---

func (in *AegisFlowProvider) DeepCopyInto(out *AegisFlowProvider) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	// ProviderSpec has a slice field (Models) that needs deep copy
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *ProviderSpec) DeepCopyInto(out *ProviderSpec) {
	*out = *in
	if in.Models != nil {
		out.Models = make([]string, len(in.Models))
		copy(out.Models, in.Models)
	}
}

func (in *AegisFlowProvider) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowProvider)
	in.DeepCopyInto(out)
	return out
}

func (in *AegisFlowProviderList) DeepCopyInto(out *AegisFlowProviderList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AegisFlowProvider, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AegisFlowProviderList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowProviderList)
	in.DeepCopyInto(out)
	return out
}

// --- AegisFlowRoute ---

func (in *AegisFlowRoute) DeepCopyInto(out *AegisFlowRoute) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *RouteSpec) DeepCopyInto(out *RouteSpec) {
	*out = *in
	out.Match = in.Match
	if in.Regions != nil {
		out.Regions = make([]RouteRegion, len(in.Regions))
		for i := range in.Regions {
			in.Regions[i].DeepCopyInto(&out.Regions[i])
		}
	}
	if in.Canary != nil {
		out.Canary = new(CanarySpec)
		in.Canary.DeepCopyInto(out.Canary)
	}
}

func (in *RouteRegion) DeepCopyInto(out *RouteRegion) {
	*out = *in
	if in.Providers != nil {
		out.Providers = make([]string, len(in.Providers))
		copy(out.Providers, in.Providers)
	}
}

func (in *CanarySpec) DeepCopyInto(out *CanarySpec) {
	*out = *in
	if in.Stages != nil {
		out.Stages = make([]int, len(in.Stages))
		copy(out.Stages, in.Stages)
	}
}

func (in *AegisFlowRoute) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowRoute)
	in.DeepCopyInto(out)
	return out
}

func (in *AegisFlowRouteList) DeepCopyInto(out *AegisFlowRouteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AegisFlowRoute, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AegisFlowRouteList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowRouteList)
	in.DeepCopyInto(out)
	return out
}

// --- AegisFlowTenant ---

func (in *AegisFlowTenant) DeepCopyInto(out *AegisFlowTenant) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *TenantSpec) DeepCopyInto(out *TenantSpec) {
	*out = *in
	if in.APIKeySecrets != nil {
		out.APIKeySecrets = make([]SecretRef, len(in.APIKeySecrets))
		copy(out.APIKeySecrets, in.APIKeySecrets)
	}
	out.RateLimit = in.RateLimit
	if in.AllowedModels != nil {
		out.AllowedModels = make([]string, len(in.AllowedModels))
		copy(out.AllowedModels, in.AllowedModels)
	}
	if in.Budget != nil {
		out.Budget = new(TenantBudgetSpec)
		in.Budget.DeepCopyInto(out.Budget)
	}
}

func (in *TenantBudgetSpec) DeepCopyInto(out *TenantBudgetSpec) {
	*out = *in
	if in.Models != nil {
		out.Models = make(map[string]ModelBudgetSpec, len(in.Models))
		for k, v := range in.Models {
			out.Models[k] = v
		}
	}
}

func (in *AegisFlowTenant) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowTenant)
	in.DeepCopyInto(out)
	return out
}

func (in *AegisFlowTenantList) DeepCopyInto(out *AegisFlowTenantList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AegisFlowTenant, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AegisFlowTenantList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowTenantList)
	in.DeepCopyInto(out)
	return out
}

// --- AegisFlowPolicy ---

func (in *AegisFlowPolicy) DeepCopyInto(out *AegisFlowPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *PolicySpec) DeepCopyInto(out *PolicySpec) {
	*out = *in
	if in.Keywords != nil {
		out.Keywords = make([]string, len(in.Keywords))
		copy(out.Keywords, in.Keywords)
	}
	if in.Patterns != nil {
		out.Patterns = make([]string, len(in.Patterns))
		copy(out.Patterns, in.Patterns)
	}
}

func (in *AegisFlowPolicy) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowPolicy)
	in.DeepCopyInto(out)
	return out
}

func (in *AegisFlowPolicyList) DeepCopyInto(out *AegisFlowPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AegisFlowPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AegisFlowPolicyList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AegisFlowPolicyList)
	in.DeepCopyInto(out)
	return out
}
