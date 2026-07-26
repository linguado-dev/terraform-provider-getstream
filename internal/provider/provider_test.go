package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/joho/godotenv"
)

// TestMain loads a local .env (if present) before running tests, so credentials
// can be kept in a gitignored .env for local acceptance runs. It walks up from
// the package directory only as far as the repository root, and uses Load (not
// Overload) so real environment variables — e.g. CI secrets — always win.
func TestMain(m *testing.M) {
	loadDotEnv()
	os.Exit(m.Run())
}

func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		if env := filepath.Join(dir, ".env"); fileExists(env) {
			_ = godotenv.Load(env)
			return
		}
		// Stop at the repository root (dir containing go.mod or .git) so a stray
		// .env in a parent directory (e.g. $HOME/.env) is never picked up.
		if fileExists(filepath.Join(dir, "go.mod")) || fileExists(filepath.Join(dir, ".git")) {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return // reached filesystem root without finding a repo root
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"getstream": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck asserts the environment required for live acceptance tests is
// present. Acceptance tests perform real (destructive) CRUD against the app the
// credentials resolve to. The terraform-plugin-testing harness already skips them
// unless TF_ACC is set; this additionally requires both credentials and guards
// against a bare `go test` with credentials in the environment silently running
// destructive tests when TF_ACC was not intended.
func testAccPreCheck(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	if firstEnv(envAPIKeyNames) == "" {
		t.Fatalf("one of %v must be set for acceptance tests", envAPIKeyNames)
	}
	if firstEnv(envAPISecretNames) == "" {
		t.Fatalf("one of %v must be set for acceptance tests", envAPISecretNames)
	}
}
