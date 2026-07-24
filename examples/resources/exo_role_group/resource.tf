resource "exo_role_group" "compliance_readers" {
  name         = "Compliance Readers"
  display_name = "Compliance Readers"
  description  = "Read-only access to compliance configuration."

  roles = [
    "View-Only Configuration",
  ]
}
