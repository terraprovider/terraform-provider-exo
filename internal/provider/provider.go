package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-exoscc/adminapi"
	"github.com/terraprovider/go-exoscc/exo"
	"github.com/terraprovider/terraform-provider-exo/internal/clients"
	"github.com/terraprovider/tf-msadmin/authschema"
)

// exoProvider implements the Exchange Online provider.
type exoProvider struct{ version string }

// New returns the provider constructor for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &exoProvider{version: version} }
}

func (p *exoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "exo"
	resp.Version = p.version
}

// providerModel embeds the shared azuread/azurerm-aligned auth block and adds
// the Exchange-specific organization routing hint.
type providerModel struct {
	authschema.Model
	Organization types.String `tfsdk:"organization"`
}

func (p *exoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	attrs := authschema.Attributes()
	attrs["organization"] = schema.StringAttribute{
		Optional:    true,
		Description: "Tenant routing domain (contoso.onmicrosoft.com); required for compliance routing under app-only.",
	}
	resp.Schema = schema.Schema{
		Description: "Manage Exchange Online via the Admin API. Authentication mirrors the AzureAD/AzureRM providers (ARM_*/AZURE_* env vars supported).",
		Attributes:  attrs,
	}
}

func (p *exoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Explicit config overlaid onto the ARM_*/AZURE_* environment.
	cfg := m.Config()

	tp, err := cfg.Build()
	if err != nil {
		resp.Diagnostics.AddError("Authentication configuration error", err.Error())
		return
	}

	// Resolve the tenant GUID from the token (the Admin API path needs the tid claim).
	tok, err := tp.Token(ctx, adminapi.EXO.Resource)
	if err != nil {
		resp.Diagnostics.AddError("Authentication failed", err.Error())
		return
	}
	tid := jwtClaim(tok, "tid")
	if tid == "" {
		resp.Diagnostics.AddError("Authentication failed", "could not read tenant id (tid) from the access token")
		return
	}

	org := m.Organization.ValueString()
	admin, err := adminapi.New(adminapi.Options{
		Cloud:        adminapi.EXO,
		TenantID:     tid,
		Tokens:       tp,
		Organization: org,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client initialisation error", err.Error())
		return
	}

	c := &clients.Client{Admin: admin, EXO: exo.New(admin), TenantID: tid}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *exoProvider) Resources(_ context.Context) []func() resource.Resource {
	return generatedResources()
}

func (p *exoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return generatedDataSources()
}

func jwtClaim(jwt, name string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if json.Unmarshal(b, &claims) != nil {
		return ""
	}
	if v, ok := claims[name].(string); ok {
		return v
	}
	return ""
}
