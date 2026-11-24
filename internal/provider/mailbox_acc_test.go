package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMailbox_shared creates a shared mailbox, imports it and expects an empty plan.
func TestAccMailbox_shared(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-mbx")
	const resourceName = "exo_mailbox.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMailboxSharedConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "shared", "true"),
					resource.TestCheckResourceAttr(resourceName, "room", "false"),
					resource.TestCheckResourceAttr("data.exo_mailbox.by_identity", "shared", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return name, nil },
				ImportStateVerifyIgnore: []string{
					"password", "room_mailbox_password", "active_sync_mailbox_policy", "address_book_policy",
					"data_encryption_policy", "default_public_folder_mailbox", "forwarding_address",
					"forwarding_smtp_address", "windows_email_address",
				},
			},
			{
				Config: testAccMailboxSharedConfig(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:      testAccMailboxConflictConfig(name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Conflicting switches`),
			},
			{
				// Flipping the type switch would delete the mailbox: the plan fails instead.
				Config:      testAccMailboxSwitchConfig(name, "shared = false"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Change would replace the object`),
			},
			{
				// Set-Mailbox -Type converts it in place.
				Config: testAccMailboxSwitchConfig(name, `type = "Room"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "type", "Room"),
					resource.TestCheckResourceAttr(resourceName, "room", "true"),
					resource.TestCheckResourceAttr(resourceName, "shared", "false"),
				),
			},
			{
				Config: testAccMailboxSwitchConfig(name, `type = "Room"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccMailboxSharedConfig(name string) string {
	return fmt.Sprintf(`
resource "exo_mailbox" "test" {
  name         = %[1]q
  display_name = %[1]q
  shared       = true
}

data "exo_mailbox" "by_identity" {
  identity = exo_mailbox.test.identity
}
`, name)
}

func testAccMailboxConflictConfig(name string) string {
	return fmt.Sprintf(`
resource "exo_mailbox" "test" {
  name   = %[1]q
  shared = true
  room   = true
}
`, name)
}

func testAccMailboxSwitchConfig(name, line string) string {
	return fmt.Sprintf(`
resource "exo_mailbox" "test" {
  name         = %[1]q
  display_name = %[1]q
  %[2]s
}
`, name, line)
}
