package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccRoleGroup_basic exercises the full lifecycle of exo_role_group against
// a live tenant: create (with a data-source read-back), in-place update, and
// import. Run with TF_ACC=1 and ARM_* credentials pointing at a disposable dev
// tenant.
func TestAccRoleGroup_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-rg")
	const resourceName = "exo_role_group.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create + data source lookup
				Config: testAccRoleGroupConfig(name, "Acc created"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "display_name", "Acc created"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "identity"),
					resource.TestCheckResourceAttr("data.exo_role_group.by_identity", "display_name", "Acc created"),
					resource.TestCheckResourceAttrPair("data.exo_role_group.by_identity", "id", resourceName, "id"),
				),
			},
			{ // import by name (role groups are addressable by name/identity, not GUID).
				// Placed before the update so the verify reads a value that has had
				// time to propagate across sessions, avoiding eventual-consistency flakes.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return name, nil },
				// System.Object-backed attributes are best-effort strings and may
				// not round-trip; ignore them in the import comparison.
				ImportStateVerifyIgnore: []string{"custom_recipient_write_scope", "managed_by", "members"},
			},
			{ // in-place update
				Config: testAccRoleGroupConfig(name, "Acc updated"),
				Check:  resource.TestCheckResourceAttr(resourceName, "display_name", "Acc updated"),
			},
		},
	})
}

func testAccRoleGroupConfig(name, displayName string) string {
	return fmt.Sprintf(`
resource "exo_role_group" "test" {
  name         = %[1]q
  description  = "acceptance test"
  display_name = %[2]q
}

data "exo_role_group" "by_identity" {
  identity = exo_role_group.test.identity
}
`, name, displayName)
}
