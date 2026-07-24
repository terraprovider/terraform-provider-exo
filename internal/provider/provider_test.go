package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories serves the exo provider to acceptance tests.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"exo": providerserver.NewProtocol6WithError(New("acc")()),
}

// testAccPreCheck skips acceptance tests unless the tenant + a credential are
// configured. Acceptance tests create and destroy real objects and must run
// against a disposable dev tenant (never one with real data).
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("ARM_TENANT_ID") == "" || os.Getenv("ARM_CLIENT_ID") == "" {
		t.Skip("set ARM_TENANT_ID, ARM_CLIENT_ID and a credential (ARM_CLIENT_SECRET / cert / OIDC) to run acceptance tests")
	}
	hasCred := os.Getenv("ARM_CLIENT_SECRET") != "" ||
		os.Getenv("ARM_CLIENT_CERTIFICATE") != "" ||
		os.Getenv("ARM_CLIENT_CERTIFICATE_PATH") != "" ||
		os.Getenv("ARM_USE_OIDC") != "" ||
		os.Getenv("ARM_OIDC_TOKEN") != "" ||
		os.Getenv("ARM_USE_CLI") != ""
	if !hasCred {
		t.Skip("no credential set (ARM_CLIENT_SECRET / ARM_CLIENT_CERTIFICATE[_PATH] / ARM_USE_OIDC / ARM_USE_CLI)")
	}
}
