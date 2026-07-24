package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-exoscc/adminapi"
	"github.com/terraprovider/go-exoscc/exo"
	"github.com/terraprovider/terraform-provider-exo/internal/clients"
)

// domainFeature is a per-domain on/off Exchange feature exposed via an
// Enable-/Disable-/Get-*Status cmdlet triple (not a New/Get/Set/Remove noun), so
// it is hand-written rather than generated. The resource's existence means the
// feature is enabled for the domain: Create enables, Delete disables, Read
// surfaces the current Get-*Status object.
type domainFeature struct {
	typeSuffix  string // e.g. "_dnssec_verified_domain" -> exo_dnssec_verified_domain
	description string
	enable      func(context.Context, *clients.Client, string) error
	disable     func(context.Context, *clients.Client, string) error
	status      func(context.Context, *clients.Client, string) (*adminapi.Result, error)
}

var (
	dnssecVerifiedDomainFeature = domainFeature{
		typeSuffix:  "_dnssec_verified_domain",
		description: "Enables DNSSEC for a verified domain (Enable-DnssecForVerifiedDomain); destroying the resource runs Disable-DnssecForVerifiedDomain.",
		enable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.EnableDnssecForVerifiedDomain(ctx, exo.EnableDnssecForVerifiedDomainParams{DomainName: d})
			return e
		},
		disable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.DisableDnssecForVerifiedDomain(ctx, exo.DisableDnssecForVerifiedDomainParams{DomainName: d})
			return e
		},
		status: func(ctx context.Context, c *clients.Client, d string) (*adminapi.Result, error) {
			return c.EXO.GetDnssecStatusForVerifiedDomain(ctx, exo.GetDnssecStatusForVerifiedDomainParams{DomainName: d})
		},
	}
	smtpDaneInboundFeature = domainFeature{
		typeSuffix:  "_smtp_dane_inbound",
		description: "Enables inbound SMTP DANE for a domain (Enable-SmtpDaneInbound); destroying the resource runs Disable-SmtpDaneInbound.",
		enable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.EnableSmtpDaneInbound(ctx, exo.EnableSmtpDaneInboundParams{DomainName: d})
			return e
		},
		disable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.DisableSmtpDaneInbound(ctx, exo.DisableSmtpDaneInboundParams{DomainName: d})
			return e
		},
		status: func(ctx context.Context, c *clients.Client, d string) (*adminapi.Result, error) {
			return c.EXO.GetSmtpDaneInboundStatus(ctx, exo.GetSmtpDaneInboundStatusParams{DomainName: d})
		},
	}
	ipv6AcceptedDomainFeature = domainFeature{
		typeSuffix:  "_ipv6_accepted_domain",
		description: "Enables IPv6 for an accepted domain (Enable-IPv6ForAcceptedDomain); destroying the resource runs Disable-IPv6ForAcceptedDomain.",
		enable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.EnableIPv6ForAcceptedDomain(ctx, exo.EnableIPv6ForAcceptedDomainParams{Domain: d})
			return e
		},
		disable: func(ctx context.Context, c *clients.Client, d string) error {
			_, e := c.EXO.DisableIPv6ForAcceptedDomain(ctx, exo.DisableIPv6ForAcceptedDomainParams{Domain: d})
			return e
		},
		status: func(ctx context.Context, c *clients.Client, d string) (*adminapi.Result, error) {
			return c.EXO.GetIPv6StatusForAcceptedDomain(ctx, exo.GetIPv6StatusForAcceptedDomainParams{Domain: d})
		},
	}
)

// NewDnssecVerifiedDomainResource manages DNSSEC for a verified domain.
func NewDnssecVerifiedDomainResource() resource.Resource {
	return &domainFeatureResource{feature: dnssecVerifiedDomainFeature}
}

// NewSmtpDaneInboundResource manages inbound SMTP DANE for a domain.
func NewSmtpDaneInboundResource() resource.Resource {
	return &domainFeatureResource{feature: smtpDaneInboundFeature}
}

// NewIPv6AcceptedDomainResource manages IPv6 for an accepted domain.
func NewIPv6AcceptedDomainResource() resource.Resource {
	return &domainFeatureResource{feature: ipv6AcceptedDomainFeature}
}

var (
	_ resource.Resource                = &domainFeatureResource{}
	_ resource.ResourceWithConfigure   = &domainFeatureResource{}
	_ resource.ResourceWithImportState = &domainFeatureResource{}
)

type domainFeatureResource struct {
	client  *clients.Client
	feature domainFeature
}

type domainFeatureModel struct {
	Domain types.String `tfsdk:"domain"`
	Status types.String `tfsdk:"status"`
}

func (r *domainFeatureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.feature.typeSuffix
}

func (r *domainFeatureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: r.feature.description + " The toggle has no boolean read-back, so out-of-band disabling is not detected as drift; the status attribute exposes the raw Get-*Status object.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:      true,
				Description:   "The domain the feature applies to. Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The Get-*Status object for the domain, encoded as JSON; decode with jsondecode().",
			},
		},
	}
}

func (r *domainFeatureResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*clients.Client)
}

func (r *domainFeatureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainFeatureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.feature.enable(ctx, r.client, plan.Domain.ValueString()); err != nil && !isAlreadyEnabled(err) {
		resp.Diagnostics.AddError("Enabling the feature failed", err.Error())
		return
	}
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainFeatureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainFeatureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if present := r.readInto(ctx, &state, &resp.Diagnostics); !present {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainFeatureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Only `domain` is configurable and it forces replacement, so there is nothing
	// to apply here — just refresh the status.
	var plan domainFeatureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *domainFeatureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainFeatureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.feature.disable(ctx, r.client, state.Domain.ValueString()); err != nil && !isNotFound(err) && !isAlreadyEnabled(err) {
		resp.Diagnostics.AddError("Disabling the feature failed", err.Error())
	}
}

func (r *domainFeatureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...)
}

// readInto loads the current status object for m.Domain. Returns false when the
// domain no longer resolves (so the resource is dropped from state).
func (r *domainFeatureResource) readInto(ctx context.Context, m *domainFeatureModel, diags *diag.Diagnostics) bool {
	res, err := r.feature.status(ctx, r.client, m.Domain.ValueString())
	if err != nil {
		if isNotFound(err) {
			return false
		}
		diags.AddError("Reading the feature status failed", err.Error())
		return false
	}
	if obj := firstObject(res.Value); obj != nil {
		m.Status = types.StringValue(toJSON(obj))
	} else {
		m.Status = types.StringValue("{}")
	}
	return true
}
