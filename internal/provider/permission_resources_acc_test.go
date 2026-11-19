package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// testAccPermissionPreCheck needs EXO_ACC_MAILBOX (an existing shared mailbox) and EXO_ACC_USER (an existing user).
func testAccPermissionPreCheck(t *testing.T) (mailbox, user string) {
	t.Helper()
	testAccPreCheck(t)
	mailbox, user = os.Getenv("EXO_ACC_MAILBOX"), os.Getenv("EXO_ACC_USER")
	if mailbox == "" || user == "" {
		t.Skip("set EXO_ACC_MAILBOX and EXO_ACC_USER to run permission acceptance tests")
	}
	return mailbox, user
}

func TestAccMailboxPermission_basic(t *testing.T) {
	mailbox, user := testAccPermissionPreCheck(t)
	const name = "exo_mailbox_permission.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMailboxPermissionConfig(mailbox, user, `["FullAccess"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", mailbox+"/"+user),
					resource.TestCheckResourceAttr(name, "access_rights.#", "1"),
					resource.TestCheckResourceAttr(name, "inheritance_type", "All"),
				),
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateId:           mailbox + "/" + user,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"automapping"},
			},
			{
				Config: testAccMailboxPermissionConfig(mailbox, user, `["FullAccess", "ReadPermission"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(name, "access_rights.#", "2"),
			},
		},
	})
}

func TestAccRecipientPermission_basic(t *testing.T) {
	mailbox, user := testAccPermissionPreCheck(t)
	const name = "exo_recipient_permission.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "exo_recipient_permission" "test" {
  identity = %q
  trustee  = %q
}
`, mailbox, user),
				Check: resource.TestCheckTypeSetElemAttr(name, "access_rights.*", "SendAs"),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     mailbox + "/" + user,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccMailboxPermissionConfig(mailbox, user, rights string) string {
	return fmt.Sprintf(`
resource "exo_mailbox_permission" "test" {
  identity      = %q
  user          = %q
  access_rights = %s
  automapping   = false
}
`, mailbox, user, rights)
}
