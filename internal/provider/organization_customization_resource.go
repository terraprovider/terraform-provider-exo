package provider

import (
	"context"
	"errors"
	"strings"

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

// organization_customization is the one hand-written resource: it wraps
// Enable-OrganizationCustomization, which is an Enable-verb bootstrap action (not
// a New/Get/Set/Remove noun) and so is not generated. It hydrates a dehydrated
// tenant so custom objects (role groups, policies, address lists, ...) can be
// created — the operation many other resources implicitly depend on. It is
// one-way: destroying the resource does not (and cannot) disable customization.

var (
	_ resource.Resource                = &organizationCustomizationResource{}
	_ resource.ResourceWithConfigure   = &organizationCustomizationResource{}
	_ resource.ResourceWithImportState = &organizationCustomizationResource{}
)

type organizationCustomizationResource struct{ client *clients.Client }

// NewOrganizationCustomizationResource enables organization customization
// (Enable-OrganizationCustomization) for the tenant.
func NewOrganizationCustomizationResource() resource.Resource {
	return &organizationCustomizationResource{}
}

type organizationCustomizationModel struct {
	ID           types.String `tfsdk:"id"`
	IsDehydrated types.Bool   `tfsdk:"is_dehydrated"`
}

func (r *organizationCustomizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_customization"
}

func (r *organizationCustomizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Enables organization customization for the tenant (Enable-OrganizationCustomization), hydrating a " +
			"dehydrated tenant so custom objects (role groups, policies, ...) can be created. Idempotent: applying it when " +
			"already customized is a no-op. Irreversible: destroying this resource only drops it from state — the tenant stays customized.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Organization identity.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_dehydrated": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the organization is still dehydrated. May remain true briefly after enabling while customization finishes provisioning.",
			},
		},
	}
}

func (r *organizationCustomizationResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*clients.Client)
}

func (r *organizationCustomizationResource) Create(ctx context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	if _, err := r.client.EXO.EnableOrganizationCustomization(ctx, exo.EnableOrganizationCustomizationParams{}); err != nil && !isAlreadyEnabled(err) {
		resp.Diagnostics.AddError("Enable-OrganizationCustomization failed", err.Error())
		return
	}
	var m organizationCustomizationModel
	if !r.read(ctx, &m, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError("Enable-OrganizationCustomization", "could not read the organization configuration after enabling")
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *organizationCustomizationResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	var m organizationCustomizationModel
	if !r.read(ctx, &m, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *organizationCustomizationResource) Update(ctx context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Nothing is configurable; just refresh is_dehydrated.
	var m organizationCustomizationModel
	if r.read(ctx, &m, &resp.Diagnostics) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}

func (r *organizationCustomizationResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Organization customization is not reversible",
		"Enable-OrganizationCustomization cannot be undone; the tenant stays customized. Removing this resource only drops it from Terraform state.",
	)
}

func (r *organizationCustomizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// read loads the current organization configuration into m. Returns false (and,
// on a real error, appends a diagnostic) when it cannot be read.
func (r *organizationCustomizationResource) read(ctx context.Context, m *organizationCustomizationModel, diags *diag.Diagnostics) bool {
	res, err := r.client.EXO.GetOrganizationConfig(ctx, exo.GetOrganizationConfigParams{})
	if err != nil {
		if isNotFound(err) {
			return false
		}
		diags.AddError("Get-OrganizationConfig failed", err.Error())
		return false
	}
	obj := firstObject(res.Value)
	if obj == nil {
		return false
	}
	m.ID = types.StringValue(firstNonEmptyStr(getString(obj, "Identity"), getString(obj, "Name"), "organization"))
	m.IsDehydrated = types.BoolValue(getBool(obj, "IsDehydrated"))
	return true
}

// isAlreadyEnabled reports whether an Enable-/Disable- error just means the
// feature is already in the desired state (or mid-provisioning) — a no-op success
// for our purposes, keeping the enable/disable resources idempotent.
func isAlreadyEnabled(err error) bool {
	var ae *adminapi.APIError
	if errors.As(err, &ae) {
		msg := strings.ToLower(ae.Message)
		return strings.Contains(msg, "already") ||
			strings.Contains(msg, "not required") ||
			strings.Contains(msg, "isn't required") ||
			strings.Contains(msg, "in progress")
	}
	return false
}
