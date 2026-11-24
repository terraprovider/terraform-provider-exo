package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMailbox_forwardingSmtpAddress sets a bare forwarding address and expects no diff after refresh or import.
func TestAccMailbox_forwardingSmtpAddress(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-fwd")
	const resourceName = "exo_mailbox.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMailboxForwardingConfig(name, "fwd-a@example.com"),
				Check:  resource.TestCheckResourceAttr(resourceName, "forwarding_smtp_address", "fwd-a@example.com"),
			},
			{
				Config: testAccMailboxForwardingConfig(name, "fwd-a@example.com"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: testAccMailboxForwardingConfig(name, "fwd-b@example.com"),
				Check:  resource.TestCheckResourceAttr(resourceName, "forwarding_smtp_address", "fwd-b@example.com"),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return name, nil },
				ImportStateCheck:  testAccCheckImportedAttr("forwarding_smtp_address", "fwd-b@example.com"),
			},
			{
				Config: testAccMailboxForwardingConfig(name, "smtp:fwd-b@example.com"),
				Check:  resource.TestCheckResourceAttr(resourceName, "forwarding_smtp_address", "smtp:fwd-b@example.com"),
			},
			{
				Config: testAccMailboxForwardingConfig(name, "smtp:fwd-b@example.com"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccCheckImportedAttr(key, want string) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("imported %d instances, want 1", len(states))
		}
		if got := states[0].Attributes[key]; got != want {
			return fmt.Errorf("imported %s = %q, want %q", key, got, want)
		}
		return nil
	}
}

func testAccMailboxForwardingConfig(name, forwarding string) string {
	return fmt.Sprintf(`
resource "exo_mailbox" "test" {
  name                    = %[1]q
  shared                  = true
  forwarding_smtp_address = %[2]q
}
`, name, forwarding)
}
