//go:build generate

package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

// Format Terraform example code used in the generated documentation.
//go:generate terraform fmt -recursive ../examples/

// Generate provider documentation from the schemas + examples into ../docs.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. --provider-name exo
