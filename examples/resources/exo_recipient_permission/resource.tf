resource "exo_recipient_permission" "support_alice" {
  identity = "support@contoso.com"
  trustee  = "alice@contoso.com"
}
