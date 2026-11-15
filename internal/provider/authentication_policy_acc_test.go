package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAuthenticationPolicy_switchOff turns a switch parameter on and back
// off (#27). AllowBasicAuthSmtp is a switch: before switches were bound as
// *bool, a false was never sent, Exchange kept True, and the post-apply plan
// was not empty. Run with TF_ACC=1 and ARM_* credentials pointing at a
// disposable dev tenant.
func TestAccAuthenticationPolicy_switchOff(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-ap")
	const resourceName = "exo_authentication_policy.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthenticationPolicyConfig(name, true),
				Check:  resource.TestCheckResourceAttr(resourceName, "allow_basic_auth_smtp", "true"),
			},
			{ // the post-apply plan check fails here if -AllowBasicAuthSmtp:$false was not sent
				Config: testAccAuthenticationPolicyConfig(name, false),
				Check:  resource.TestCheckResourceAttr(resourceName, "allow_basic_auth_smtp", "false"),
			},
		},
	})
}

func testAccAuthenticationPolicyConfig(name string, smtp bool) string {
	return fmt.Sprintf(`
resource "exo_authentication_policy" "test" {
  name                  = %[1]q
  allow_basic_auth_smtp = %[2]t
}
`, name, smtp)
}
