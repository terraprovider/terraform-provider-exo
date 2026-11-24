package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccArcConfig_unconfigured needs a tenant where Set-ArcConfig never ran; it makes no writes.
func TestAccArcConfig_unconfigured(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "exo_arc_config" "this" {}`,
				Check:  resource.TestCheckResourceAttr("data.exo_arc_config.this", "arc_trusted_sealers.#", "0"),
			},
			{
				Config: `
import {
  to = exo_arc_config.this
  id = "default"
}

resource "exo_arc_config" "this" {}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("exo_arc_config.this", "arc_trusted_sealers.#", "0"),
			},
		},
	})
}
