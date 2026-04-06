package resource

import "testing"

func TestResourceTypes(t *testing.T) {
	types := []ResourceType{
		ResourceRepo, ResourceBranch, ResourceFile, ResourcePR,
		ResourceWorkflow, ResourceDatabase, ResourceSchema, ResourceTable,
		ResourceHost, ResourceFilesystem, ResourceNetwork, ResourceEndpoint,
		ResourceBucket, ResourceNamespace, ResourceCluster, ResourceAccount,
	}
	seen := make(map[ResourceType]bool)
	for _, rt := range types {
		if rt == "" {
			t.Error("resource type must not be empty")
		}
		if seen[rt] {
			t.Errorf("duplicate resource type: %s", rt)
		}
		seen[rt] = true
	}
}

func TestSensitivityRanking(t *testing.T) {
	levels := []Sensitivity{SensitivityPublic, SensitivityInternal, SensitivityConfidential, SensitivitySecret}
	for i := 1; i < len(levels); i++ {
		if sensitivityRank(levels[i]) <= sensitivityRank(levels[i-1]) {
			t.Errorf("expected %s > %s in rank", levels[i], levels[i-1])
		}
	}
}

func TestResourcePathString(t *testing.T) {
	r := Resource{
		Type: ResourceTable,
		Path: []string{"mydb", "public", "users"},
	}
	if got := r.PathString(); got != "mydb/public/users" {
		t.Errorf("PathString() = %q, want %q", got, "mydb/public/users")
	}

	empty := Resource{}
	if got := empty.PathString(); got != "" {
		t.Errorf("empty PathString() = %q, want empty", got)
	}
}

func TestResourceCreation(t *testing.T) {
	r := Resource{
		Type:        ResourceRepo,
		Provider:    "github",
		Environment: "prod",
		Path:        []string{"myorg", "myrepo"},
		Sensitivity: SensitivityInternal,
		Properties:  map[string]string{"visibility": "private"},
	}

	if r.Type != ResourceRepo {
		t.Errorf("Type = %s, want repo", r.Type)
	}
	if r.Provider != "github" {
		t.Errorf("Provider = %s, want github", r.Provider)
	}
	if r.PathString() != "myorg/myrepo" {
		t.Errorf("PathString() = %s, want myorg/myrepo", r.PathString())
	}
	if r.Properties["visibility"] != "private" {
		t.Error("Properties not set correctly")
	}
}

func TestVerbConstants(t *testing.T) {
	verbs := []Verb{VerbRead, VerbCreate, VerbUpdate, VerbDelete, VerbDeploy, VerbApprove, VerbExecute, VerbGrant}
	seen := make(map[Verb]bool)
	for _, v := range verbs {
		if v == "" {
			t.Error("verb must not be empty")
		}
		if seen[v] {
			t.Errorf("duplicate verb: %s", v)
		}
		seen[v] = true
	}
}
