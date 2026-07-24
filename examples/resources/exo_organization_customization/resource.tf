# Hydrate a dehydrated tenant so custom objects (role groups, policies, ...) can
# be created. Other resources should depend on this on a fresh tenant.
resource "exo_organization_customization" "this" {}

resource "exo_role_group" "example" {
  name       = "tf-managed"
  depends_on = [exo_organization_customization.this]
}
