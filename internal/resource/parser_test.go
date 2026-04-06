package resource

import "testing"

func TestParseGitHubResourceRepo(t *testing.T) {
	res := ParseGitHubResource("github.list_repos", "myorg/myrepo", map[string]any{})
	if res.Type != ResourceRepo {
		t.Errorf("Type = %s, want repo", res.Type)
	}
	if res.Provider != "github" {
		t.Errorf("Provider = %s, want github", res.Provider)
	}
	if len(res.Path) != 2 || res.Path[0] != "myorg" || res.Path[1] != "myrepo" {
		t.Errorf("Path = %v, want [myorg myrepo]", res.Path)
	}
}

func TestParseGitHubResourcePR(t *testing.T) {
	params := map[string]any{"pull_number": "42"}
	res := ParseGitHubResource("github.create_pull_request", "myorg/myrepo", params)
	if res.Type != ResourcePR {
		t.Errorf("Type = %s, want pull_request", res.Type)
	}
	if res.Properties["pull_number"] != "42" {
		t.Errorf("pull_number = %s, want 42", res.Properties["pull_number"])
	}
}

func TestParseGitHubResourceBranch(t *testing.T) {
	params := map[string]any{"branch": "main"}
	res := ParseGitHubResource("github.create_branch", "myorg/myrepo", params)
	if res.Type != ResourceBranch {
		t.Errorf("Type = %s, want branch", res.Type)
	}
	if len(res.Path) != 3 || res.Path[2] != "main" {
		t.Errorf("Path = %v, want [myorg myrepo main]", res.Path)
	}
	if res.Environment != "prod" {
		t.Errorf("Environment = %s, want prod (main branch)", res.Environment)
	}
}

func TestParseGitHubResourceFile(t *testing.T) {
	params := map[string]any{"path": "src/main.go"}
	res := ParseGitHubResource("github.get_file_contents", "myorg/myrepo", params)
	if res.Type != ResourceFile {
		t.Errorf("Type = %s, want file", res.Type)
	}
}

func TestParseGitHubResourceWorkflow(t *testing.T) {
	res := ParseGitHubResource("github.trigger_workflow", "myorg/myrepo", map[string]any{})
	if res.Type != ResourceWorkflow {
		t.Errorf("Type = %s, want workflow", res.Type)
	}
}

func TestParseGitHubEnvironmentDev(t *testing.T) {
	res := ParseGitHubResource("github.list_repos", "myorg/myrepo", map[string]any{
		"branch": "feature/foo",
	})
	if res.Environment != "dev" {
		t.Errorf("Environment = %s, want dev for feature branch", res.Environment)
	}
}

func TestParseSQLResourceThreePart(t *testing.T) {
	res := ParseSQLResource("query", "mydb.public.users")
	if res.Type != ResourceTable {
		t.Errorf("Type = %s, want table", res.Type)
	}
	if res.Provider != "postgres" {
		t.Errorf("Provider = %s, want postgres", res.Provider)
	}
	if len(res.Path) != 3 || res.Path[0] != "mydb" || res.Path[1] != "public" || res.Path[2] != "users" {
		t.Errorf("Path = %v, want [mydb public users]", res.Path)
	}
	if res.Sensitivity != SensitivityConfidential {
		t.Errorf("Sensitivity = %s, want confidential", res.Sensitivity)
	}
}

func TestParseSQLResourceTwoPart(t *testing.T) {
	res := ParseSQLResource("query", "mydb.users")
	if res.Type != ResourceTable {
		t.Errorf("Type = %s, want table", res.Type)
	}
	if len(res.Path) != 2 {
		t.Errorf("Path = %v, want 2 segments", res.Path)
	}
}

func TestParseSQLResourceSinglePart(t *testing.T) {
	res := ParseSQLResource("list_tables", "mydb")
	if res.Type != ResourceDatabase {
		t.Errorf("Type = %s, want database", res.Type)
	}
}

func TestParseSQLResourceSchemaFromTool(t *testing.T) {
	res := ParseSQLResource("describe_schema", "public")
	if res.Type != ResourceSchema {
		t.Errorf("Type = %s, want schema", res.Type)
	}
}

func TestParseShellResourceFilesystem(t *testing.T) {
	res := ParseShellResource("cat", "/etc/passwd")
	if res.Type != ResourceFilesystem {
		t.Errorf("Type = %s, want filesystem", res.Type)
	}
	if res.Provider != "shell" {
		t.Errorf("Provider = %s, want shell", res.Provider)
	}
	if len(res.Path) < 2 || res.Path[0] != "etc" {
		t.Errorf("Path = %v, want path starting with etc", res.Path)
	}
}

func TestParseShellResourceNetwork(t *testing.T) {
	res := ParseShellResource("curl", "https://api.example.com")
	if res.Type != ResourceNetwork {
		t.Errorf("Type = %s, want network", res.Type)
	}
}

func TestParseShellResourceSudo(t *testing.T) {
	res := ParseShellResource("sudo rm", "/var/log/app.log")
	if res.Sensitivity != SensitivitySecret {
		t.Errorf("Sensitivity = %s, want secret for sudo", res.Sensitivity)
	}
}

func TestParseShellResourceRm(t *testing.T) {
	res := ParseShellResource("rm -rf", "/tmp/data")
	if res.Sensitivity != SensitivityConfidential {
		t.Errorf("Sensitivity = %s, want confidential for rm", res.Sensitivity)
	}
}

func TestParseHTTPResource(t *testing.T) {
	res := ParseHTTPResource("GET", "https://api.stripe.com/v1/charges")
	if res.Type != ResourceEndpoint {
		t.Errorf("Type = %s, want endpoint", res.Type)
	}
	if res.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", res.Provider)
	}
	if res.Properties["method"] != "GET" {
		t.Errorf("method = %s, want GET", res.Properties["method"])
	}
	if len(res.Path) < 2 || res.Path[0] != "api.stripe.com" {
		t.Errorf("Path = %v, want [api.stripe.com v1 charges]", res.Path)
	}
}

func TestParseHTTPResourceGitHub(t *testing.T) {
	res := ParseHTTPResource("POST", "https://api.github.com/repos/myorg/myrepo/pulls")
	if res.Provider != "github" {
		t.Errorf("Provider = %s, want github", res.Provider)
	}
}

func TestParseHTTPResourceSlack(t *testing.T) {
	res := ParseHTTPResource("POST", "https://slack.com/api/chat.postMessage")
	if res.Provider != "slack" {
		t.Errorf("Provider = %s, want slack", res.Provider)
	}
}

func TestParseHTTPResourceProdEnvironment(t *testing.T) {
	res := ParseHTTPResource("DELETE", "https://api.prod.example.com/users/123")
	if res.Environment != "prod" {
		t.Errorf("Environment = %s, want prod", res.Environment)
	}
}

func TestParseHTTPResourceStagingEnvironment(t *testing.T) {
	res := ParseHTTPResource("GET", "https://staging.example.com/health")
	if res.Environment != "staging" {
		t.Errorf("Environment = %s, want staging", res.Environment)
	}
}

func TestParseHTTPResourceInvalidURL(t *testing.T) {
	res := ParseHTTPResource("GET", "://invalid")
	if res.Type != ResourceEndpoint {
		t.Errorf("Type = %s, want endpoint for invalid URL", res.Type)
	}
}
