resource "exo_mailbox_permission" "support_alice" {
  identity      = "support@contoso.com"
  user          = "alice@contoso.com"
  access_rights = ["FullAccess"]
  automapping   = false
}
