package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-exoscc/exo"
	"github.com/terraprovider/go-msadmin/consistency"
	"github.com/terraprovider/terraform-provider-exo/internal/clients"
	"github.com/terraprovider/tf-msadmin/resourcex"
)

var permissionConsistency = consistency.Config{}

const selfPrincipal = `NT AUTHORITY\SELF`

var (
	_ resource.Resource                = &mailboxPermissionGrantResource{}
	_ resource.ResourceWithConfigure   = &mailboxPermissionGrantResource{}
	_ resource.ResourceWithImportState = &mailboxPermissionGrantResource{}
	_ resource.Resource                = &recipientPermissionGrantResource{}
	_ resource.ResourceWithConfigure   = &recipientPermissionGrantResource{}
	_ resource.ResourceWithImportState = &recipientPermissionGrantResource{}
)

// NewMailboxPermissionResource manages one user's explicit permissions on a mailbox (Add-/Remove-MailboxPermission).
func NewMailboxPermissionResource() resource.Resource { return &mailboxPermissionGrantResource{} }

// NewRecipientPermissionResource manages one trustee's SendAs permission on a recipient (Add-/Remove-RecipientPermission).
func NewRecipientPermissionResource() resource.Resource { return &recipientPermissionGrantResource{} }

type mailboxPermissionGrantResource struct{ client *clients.Client }

type mailboxPermissionGrantModel struct {
	ID              types.String `tfsdk:"id"`
	Identity        types.String `tfsdk:"identity"`
	User            types.String `tfsdk:"user"`
	AccessRights    types.Set    `tfsdk:"access_rights"`
	InheritanceType types.String `tfsdk:"inheritance_type"`
	AutoMapping     types.Bool   `tfsdk:"automapping"`
}

func (r *mailboxPermissionGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mailbox_permission"
}

func (r *mailboxPermissionGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one user's explicit permissions on a mailbox, e.g. FullAccess on a shared mailbox. " +
			"Create runs Add-MailboxPermission, destroy runs Remove-MailboxPermission, and read uses Get-MailboxPermission -User, " +
			"ignoring inherited, deny and NT AUTHORITY\\SELF entries. Import with `<identity>/<user>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "`<identity>/<user>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"identity": schema.StringAttribute{
				Required:      true,
				Description:   "The mailbox (-Identity). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user": schema.StringAttribute{
				Required:      true,
				Description:   "The user or group granted the permissions (-User). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"access_rights": schema.SetAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "Maps to -AccessRights. Allowed values: ChangeOwner, ChangePermission, DeleteItem, ExternalAccount, FullAccess, ReadPermission. " +
					"Added and removed rights are updated in place.",
			},
			"inheritance_type": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("All"),
				Description:   "Maps to -InheritanceType. Allowed values: None, All, Children, Descendents, SelfAndChildren. Defaults to All. Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"automapping": schema.BoolAttribute{
				Optional: true,
				Description: "Maps to -AutoMapping, which only Add-MailboxPermission takes and Get-MailboxPermission does not return. " +
					"Changing it forces replacement, except after import, where it is only recorded.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplaceIf(replaceUnlessImported,
					"Changing automapping re-creates the grant.", "Changing automapping re-creates the grant.")},
			},
		},
	}
}

func replaceUnlessImported(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.IsNull()
}

func (r *mailboxPermissionGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*clients.Client)
}

func (r *mailboxPermissionGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailboxPermissionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	rights := toStringSlice(ctx, plan.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.add(ctx, plan, rights); err != nil {
		resp.Diagnostics.AddError("Add-MailboxPermission failed", err.Error())
		return
	}
	plan.ID = types.StringValue(permissionID(plan.Identity, plan.User))
	if !r.refresh(ctx, &plan, rights, &resp.Diagnostics) && !resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Mailbox permission not found after Add-MailboxPermission",
			fmt.Sprintf("Get-MailboxPermission -Identity %s -User %s returned no explicit grant", plan.Identity.ValueString(), plan.User.ValueString()))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailboxPermissionGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailboxPermissionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.refresh(ctx, &state, nil, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mailboxPermissionGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailboxPermissionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	want := toStringSlice(ctx, plan.AccessRights, &resp.Diagnostics)
	have := toStringSlice(ctx, state.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	added, removed := diffRights(have, want)
	if len(added) > 0 {
		if err := r.add(ctx, plan, added); err != nil {
			resp.Diagnostics.AddError("Add-MailboxPermission failed", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		if err := r.remove(ctx, state, removed); err != nil && !isNotFound(err) {
			resp.Diagnostics.AddError("Remove-MailboxPermission failed", err.Error())
			return
		}
	}
	if !r.refresh(ctx, &plan, want, &resp.Diagnostics) && !resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Mailbox permission not found after update", "Get-MailboxPermission returned no explicit grant")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailboxPermissionGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailboxPermissionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	rights := toStringSlice(ctx, state.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.remove(ctx, state, rights); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Remove-MailboxPermission failed", err.Error())
	}
}

func (r *mailboxPermissionGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	identity, user, ok := splitPermissionID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <identity>/<user>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("identity"), identity)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user"), user)...)
}

func (r *mailboxPermissionGrantResource) add(ctx context.Context, m mailboxPermissionGrantModel, rights []string) error {
	p := exo.AddMailboxPermissionParams{
		Identity:     m.Identity.ValueString(),
		User:         m.User.ValueString(),
		AccessRights: rights,
		AutoMapping:  m.AutoMapping.ValueBoolPointer(),
	}
	if v := m.InheritanceType.ValueString(); v != "" {
		p.InheritanceType = v
	}
	_, err := resourcex.RetryWriteCall(ctx, permissionConsistency, r.client.EXO.AddMailboxPermission, p, isNotFound)
	return err
}

func (r *mailboxPermissionGrantResource) remove(ctx context.Context, m mailboxPermissionGrantModel, rights []string) error {
	p := exo.RemoveMailboxPermissionParams{
		Identity:     m.Identity.ValueString(),
		User:         m.User.ValueString(),
		AccessRights: rights,
	}
	if v := m.InheritanceType.ValueString(); v != "" {
		p.InheritanceType = v
	}
	_, err := r.client.EXO.RemoveMailboxPermission(ctx, p)
	return err
}

// refresh reads the user's explicit grant into m, waiting until it holds want when want is set.
func (r *mailboxPermissionGrantResource) refresh(ctx context.Context, m *mailboxPermissionGrantModel, want []string, diags *diag.Diagnostics) bool {
	identity, user := m.Identity.ValueString(), m.User.ValueString()
	get := func(ctx context.Context) (permissionGrant, bool, error) {
		res, err := r.client.EXO.GetMailboxPermission(ctx, exo.GetMailboxPermissionParams{Identity: identity, User: user})
		if err != nil {
			if isNotFound(err) {
				return permissionGrant{}, false, nil
			}
			return permissionGrant{}, false, err
		}
		g := explicitGrant(res.Value, "User", mailboxPermissionAllowed)
		return g, len(g.rights) > 0, nil
	}
	g, present, err := resourcex.LoadUntil(ctx, permissionConsistency, get, holdsRights(want))
	if err != nil {
		diags.AddError("Get-MailboxPermission failed", err.Error())
		return false
	}
	if !present {
		return false
	}
	m.ID = types.StringValue(permissionID(m.Identity, m.User))
	m.AccessRights = keepRights(ctx, m.AccessRights, g.rights, diags)
	if g.inheritance != "" && !strings.EqualFold(m.InheritanceType.ValueString(), g.inheritance) {
		m.InheritanceType = types.StringValue(g.inheritance)
	}
	return true
}

type recipientPermissionGrantResource struct{ client *clients.Client }

type recipientPermissionGrantModel struct {
	ID           types.String `tfsdk:"id"`
	Identity     types.String `tfsdk:"identity"`
	Trustee      types.String `tfsdk:"trustee"`
	AccessRights types.Set    `tfsdk:"access_rights"`
}

func (r *recipientPermissionGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_recipient_permission"
}

func (r *recipientPermissionGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one trustee's SendAs permission on a recipient, e.g. a shared mailbox. " +
			"Create runs Add-RecipientPermission, destroy runs Remove-RecipientPermission, and read uses Get-RecipientPermission -Trustee, " +
			"ignoring inherited, deny and NT AUTHORITY\\SELF entries. Import with `<identity>/<trustee>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "`<identity>/<trustee>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"identity": schema.StringAttribute{
				Required:      true,
				Description:   "The recipient (-Identity). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"trustee": schema.StringAttribute{
				Required:      true,
				Description:   "The user or group granted the permission (-Trustee). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"access_rights": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{types.StringValue("SendAs")})),
				Description: "Maps to -AccessRights. The only allowed value is SendAs, the default.",
			},
		},
	}
}

func (r *recipientPermissionGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*clients.Client)
}

func (r *recipientPermissionGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan recipientPermissionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	rights := toStringSlice(ctx, plan.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.add(ctx, plan, rights); err != nil {
		resp.Diagnostics.AddError("Add-RecipientPermission failed", err.Error())
		return
	}
	if !r.refresh(ctx, &plan, rights, &resp.Diagnostics) && !resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Recipient permission not found after Add-RecipientPermission",
			fmt.Sprintf("Get-RecipientPermission -Identity %s -Trustee %s returned no explicit grant", plan.Identity.ValueString(), plan.Trustee.ValueString()))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *recipientPermissionGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state recipientPermissionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.refresh(ctx, &state, nil, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *recipientPermissionGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state recipientPermissionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	want := toStringSlice(ctx, plan.AccessRights, &resp.Diagnostics)
	have := toStringSlice(ctx, state.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	added, removed := diffRights(have, want)
	if len(added) > 0 {
		if err := r.add(ctx, plan, added); err != nil {
			resp.Diagnostics.AddError("Add-RecipientPermission failed", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		if err := r.remove(ctx, state, removed); err != nil && !isNotFound(err) {
			resp.Diagnostics.AddError("Remove-RecipientPermission failed", err.Error())
			return
		}
	}
	if !r.refresh(ctx, &plan, want, &resp.Diagnostics) && !resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Recipient permission not found after update", "Get-RecipientPermission returned no explicit grant")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *recipientPermissionGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state recipientPermissionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	rights := toStringSlice(ctx, state.AccessRights, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.remove(ctx, state, rights); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Remove-RecipientPermission failed", err.Error())
	}
}

func (r *recipientPermissionGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	identity, trustee, ok := splitPermissionID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <identity>/<trustee>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("identity"), identity)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("trustee"), trustee)...)
}

func (r *recipientPermissionGrantResource) add(ctx context.Context, m recipientPermissionGrantModel, rights []string) error {
	p := exo.AddRecipientPermissionParams{Identity: m.Identity.ValueString(), Trustee: m.Trustee.ValueString(), AccessRights: rights}
	_, err := resourcex.RetryWriteCall(ctx, permissionConsistency, r.client.EXO.AddRecipientPermission, p, isNotFound)
	return err
}

func (r *recipientPermissionGrantResource) remove(ctx context.Context, m recipientPermissionGrantModel, rights []string) error {
	p := exo.RemoveRecipientPermissionParams{Identity: m.Identity.ValueString(), Trustee: m.Trustee.ValueString(), AccessRights: rights}
	_, err := r.client.EXO.RemoveRecipientPermission(ctx, p)
	return err
}

// refresh reads the trustee's explicit grant into m, waiting until it holds want when want is set.
func (r *recipientPermissionGrantResource) refresh(ctx context.Context, m *recipientPermissionGrantModel, want []string, diags *diag.Diagnostics) bool {
	identity, trustee := m.Identity.ValueString(), m.Trustee.ValueString()
	get := func(ctx context.Context) (permissionGrant, bool, error) {
		res, err := r.client.EXO.GetRecipientPermission(ctx, exo.GetRecipientPermissionParams{Identity: identity, Trustee: trustee})
		if err != nil {
			if isNotFound(err) {
				return permissionGrant{}, false, nil
			}
			return permissionGrant{}, false, err
		}
		g := explicitGrant(res.Value, "Trustee", recipientPermissionAllowed)
		return g, len(g.rights) > 0, nil
	}
	g, present, err := resourcex.LoadUntil(ctx, permissionConsistency, get, holdsRights(want))
	if err != nil {
		diags.AddError("Get-RecipientPermission failed", err.Error())
		return false
	}
	if !present {
		return false
	}
	m.ID = types.StringValue(permissionID(m.Identity, m.Trustee))
	m.AccessRights = keepRights(ctx, m.AccessRights, g.rights, diags)
	return true
}

// keepRights returns prior when it names the same rights as read, ignoring case, otherwise read.
func keepRights(ctx context.Context, prior types.Set, read []string, diags *diag.Diagnostics) types.Set {
	if !prior.IsNull() && !prior.IsUnknown() {
		added, removed := diffRights(toStringSlice(ctx, prior, diags), read)
		if len(added) == 0 && len(removed) == 0 {
			return prior
		}
	}
	return stringSetValue(ctx, read)
}

// permissionGrant is the union of a principal's explicit permission entries.
type permissionGrant struct {
	rights      []string
	inheritance string
}

func mailboxPermissionAllowed(e map[string]any) bool { return !getBool(e, "Deny") }

func recipientPermissionAllowed(e map[string]any) bool {
	v := getString(e, "AccessControlType")
	return v == "" || strings.EqualFold(v, "Allow")
}

// explicitGrant merges the entries that are not inherited, not denied and not NT AUTHORITY\SELF.
func explicitGrant(entries []map[string]any, principalKey string, allowed func(map[string]any) bool) permissionGrant {
	seen := map[string]bool{}
	var g permissionGrant
	for _, e := range entries {
		if getBool(e, "IsInherited") || !allowed(e) || strings.EqualFold(getString(e, principalKey), selfPrincipal) {
			continue
		}
		for _, r := range accessRights(e) {
			if !seen[strings.ToLower(r)] {
				seen[strings.ToLower(r)] = true
				g.rights = append(g.rights, r)
			}
		}
		if g.inheritance == "" {
			g.inheritance = getString(e, "InheritanceType")
		}
	}
	sort.Strings(g.rights)
	return g
}

// accessRights reads AccessRights as a list, splitting a comma-separated string.
func accessRights(e map[string]any) []string {
	var out []string
	for _, v := range getStringSlice(e, "AccessRights") {
		for _, r := range strings.Split(v, ",") {
			if r = strings.TrimSpace(r); r != "" {
				out = append(out, r)
			}
		}
	}
	return out
}

// holdsRights reports whether a grant includes every right in want; nil when want is empty.
func holdsRights(want []string) func(permissionGrant) bool {
	if len(want) == 0 {
		return nil
	}
	return func(g permissionGrant) bool {
		added, removed := diffRights(g.rights, want)
		return len(added) == 0 && len(removed) == 0
	}
}

// diffRights returns the rights in want but not in have, and the rights in have but not in want.
func diffRights(have, want []string) (added, removed []string) {
	in := func(s []string, v string) bool {
		for _, x := range s {
			if strings.EqualFold(x, v) {
				return true
			}
		}
		return false
	}
	for _, w := range want {
		if !in(have, w) {
			added = append(added, w)
		}
	}
	for _, h := range have {
		if !in(want, h) {
			removed = append(removed, h)
		}
	}
	return added, removed
}

func permissionID(identity, principal types.String) string {
	return identity.ValueString() + "/" + principal.ValueString()
}

// splitPermissionID splits an `<identity>/<principal>` import id at its last slash.
func splitPermissionID(id string) (identity, principal string, ok bool) {
	i := strings.LastIndex(id, "/")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}
