# terraform-provider-exo

[![Test](https://github.com/terraprovider/terraform-provider-exo/actions/workflows/test.yml/badge.svg)](https://github.com/terraprovider/terraform-provider-exo/actions/workflows/test.yml)
[![Build](https://github.com/terraprovider/terraform-provider-exo/actions/workflows/build.yml/badge.svg)](https://github.com/terraprovider/terraform-provider-exo/actions/workflows/build.yml)

A Terraform / OpenTofu provider for **Exchange Online**, driven by the same
Admin API the `ExchangeOnlineManagement` PowerShell module uses. Resources are
**generated from the module's own cmdlet catalog** (via
[`tf-msadmin/genframework`](https://github.com/terraprovider/tf-msadmin) and the
[`go-exoscc`](https://github.com/terraprovider/go-exoscc) bindings), so the surface
stays in lock-step with the service.

> Not affiliated with or endorsed by Microsoft.

## Authentication

The provider's configuration is aligned field-for-field with the
`hashicorp/azuread` and `azurerm` providers — the same attribute names, the same
`ARM_*` / `AZURE_*` environment variables, and every OIDC / workload-identity
flavour (GitHub Actions, Azure DevOps, generic). Supported methods: client
secret, client certificate (PEM or PKCS#12), OIDC federation, Azure CLI, and
managed identity.

```hcl
provider "exo" {
  tenant_id     = "00000000-0000-0000-0000-000000000000"
  client_id     = "11111111-1111-1111-1111-111111111111"
  client_secret = var.client_secret
}
```

App-only auth requires the app to be granted `Exchange.ManageAsApp` and an Entra
directory role; Purview/compliance routing additionally needs `organization`.

## Development

```bash
go build ./...                       # build
go test ./...                        # test
go run ./cmd/gen-tf                  # regenerate all resources from the catalog
go run ./cmd/gen-tf -noun RoleGroup  # regenerate one
cd tools && go generate ./...        # regenerate docs (tfplugindocs)
```

Resource files under `internal/provider/*_resource.go` are generated
(`DO NOT EDIT`); change the generator, not the output. Local development against
sibling module checkouts uses the git-ignored `go.work`.

## License

MIT — see [LICENSE](LICENSE).
