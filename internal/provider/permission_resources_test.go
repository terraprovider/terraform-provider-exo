package provider

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/terraprovider/go-msadmin/consistency"
)

func TestExplicitGrantSkipsInheritedDenyAndSelf(t *testing.T) {
	entries := []map[string]any{
		{"User": `NT AUTHORITY\SELF`, "AccessRights": []any{"FullAccess", "ReadPermission"}, "IsInherited": false},
		{"User": "a@contoso.com", "AccessRights": []any{"FullAccess"}, "IsInherited": false, "Deny": false, "InheritanceType": "All"},
		{"User": "a@contoso.com", "AccessRights": "ReadPermission, FullAccess", "IsInherited": false, "InheritanceType": "All"},
		{"User": "a@contoso.com", "AccessRights": []any{"DeleteItem"}, "IsInherited": true},
		{"User": "a@contoso.com", "AccessRights": []any{"ChangeOwner"}, "Deny": true},
	}
	g := explicitGrant(entries, "User", mailboxPermissionAllowed)
	if want := []string{"FullAccess", "ReadPermission"}; !reflect.DeepEqual(g.rights, want) || g.inheritance != "All" {
		t.Errorf("explicitGrant = %v %q, want %v All", g.rights, g.inheritance, want)
	}
	recipients := []map[string]any{
		{"Trustee": `NT AUTHORITY\SELF`, "AccessRights": []any{"SendAs"}, "AccessControlType": "Allow"},
		{"Trustee": "b@contoso.com", "AccessRights": []any{"SendAs"}, "AccessControlType": "Deny"},
	}
	if g := explicitGrant(recipients, "Trustee", recipientPermissionAllowed); len(g.rights) != 0 {
		t.Errorf("self and deny entries must not count: %v", g.rights)
	}
}

func TestDiffRights(t *testing.T) {
	added, removed := diffRights([]string{"FullAccess", "ReadPermission"}, []string{"fullaccess", "DeleteItem"})
	if !reflect.DeepEqual(added, []string{"DeleteItem"}) || !reflect.DeepEqual(removed, []string{"ReadPermission"}) {
		t.Errorf("diffRights = +%v -%v", added, removed)
	}
}

func TestSplitPermissionID(t *testing.T) {
	cases := map[string][3]string{
		"shared@contoso.com/a@contoso.com": {"shared@contoso.com", "a@contoso.com", "ok"},
		"Sales/Team/a@contoso.com":         {"Sales/Team", "a@contoso.com", "ok"},
		"no-slash":                         {"", "", ""},
		"/a@contoso.com":                   {"", "", ""},
		"shared@contoso.com/":              {"", "", ""},
	}
	for id, want := range cases {
		i, p, ok := splitPermissionID(id)
		if i != want[0] || p != want[1] || ok != (want[2] == "ok") {
			t.Errorf("splitPermissionID(%q) = %q %q %v", id, i, p, ok)
		}
	}
}

// fakeGrants is an in-memory permission table behind a fake Admin API.
type fakeGrants struct {
	mu     sync.Mutex
	rights map[string][]string
}

func (f *fakeGrants) respond(cmdlet string, p map[string]any) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	principal := "User"
	if strings.Contains(cmdlet, "Recipient") {
		principal = "Trustee"
	}
	key, _ := p["Identity"].(string)
	who, _ := p[principal].(string)
	key += "|" + who
	var asked []string
	if v, ok := p["AccessRights"].([]any); ok {
		for _, r := range v {
			asked = append(asked, r.(string))
		}
	}
	switch {
	case strings.HasPrefix(cmdlet, "Add-"):
		added, _ := diffRights(f.rights[key], asked)
		f.rights[key] = append(f.rights[key], added...)
	case strings.HasPrefix(cmdlet, "Remove-"):
		_, kept := diffRights(f.rights[key], asked)
		f.rights[key] = kept
	case strings.HasPrefix(cmdlet, "Get-"):
		out := []map[string]any{{principal: `NT AUTHORITY\SELF`, "AccessRights": []any{"FullAccess"}, "IsInherited": false}}
		if rs := f.rights[key]; len(rs) > 0 {
			var v []any
			for _, r := range rs {
				v = append(v, r)
			}
			out = append(out, map[string]any{principal: who, "AccessRights": v, "IsInherited": false, "InheritanceType": "All", "AccessControlType": "Allow"})
		}
		return out
	}
	return nil
}

func fastPermissionConsistency(t *testing.T) {
	t.Helper()
	prev := permissionConsistency
	permissionConsistency = consistency.Config{Attempts: 1, Delay: time.Millisecond}
	t.Cleanup(func() { permissionConsistency = prev })
}

func TestMailboxPermissionLifecycle(t *testing.T) {
	fastPermissionConsistency(t)
	ctx := context.Background()
	grants := &fakeGrants{rights: map[string][]string{}}
	c, fake := newFakeClient(t, grants.respond)
	r := &mailboxPermissionGrantResource{client: c}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	s := sr.Schema

	planned := mailboxPermissionGrantModel{
		ID:              types.StringUnknown(),
		Identity:        types.StringValue("shared@contoso.com"),
		User:            types.StringValue("a@contoso.com"),
		AccessRights:    stringSetValue(ctx, []string{"FullAccess"}),
		InheritanceType: types.StringValue("All"),
		AutoMapping:     types.BoolValue(false),
	}
	plan := tfsdk.Plan{Schema: s}
	plan.Set(ctx, &planned)
	cresp := resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &cresp)
	if cresp.Diagnostics.HasError() {
		t.Fatal(cresp.Diagnostics)
	}
	var got mailboxPermissionGrantModel
	cresp.State.Get(ctx, &got)
	if got.ID.ValueString() != "shared@contoso.com/a@contoso.com" {
		t.Errorf("id = %s", got.ID)
	}
	add := fake.Calls()[0]
	if add.Cmdlet != "Add-MailboxPermission" || add.Params["AutoMapping"] != false || add.Params["InheritanceType"] != "All" {
		t.Errorf("first call = %+v", add)
	}

	planned.ID = got.ID
	planned.AccessRights = stringSetValue(ctx, []string{"ReadPermission"})
	plan.Set(ctx, &planned)
	uresp := resource.UpdateResponse{State: cresp.State}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: cresp.State}, &uresp)
	if uresp.Diagnostics.HasError() {
		t.Fatal(uresp.Diagnostics)
	}
	if rs := grants.rights["shared@contoso.com|a@contoso.com"]; !reflect.DeepEqual(rs, []string{"ReadPermission"}) {
		t.Errorf("rights after update = %v", rs)
	}

	r.Delete(ctx, resource.DeleteRequest{State: uresp.State}, &resource.DeleteResponse{})
	rresp := resource.ReadResponse{State: uresp.State}
	r.Read(ctx, resource.ReadRequest{State: uresp.State}, &rresp)
	if rresp.Diagnostics.HasError() || !rresp.State.Raw.IsNull() {
		t.Errorf("a removed grant must leave state (diags %v)", rresp.Diagnostics)
	}
}

func TestMailboxPermissionImportReadsGrant(t *testing.T) {
	fastPermissionConsistency(t)
	ctx := context.Background()
	grants := &fakeGrants{rights: map[string][]string{"shared@contoso.com|a@contoso.com": {"FullAccess"}}}
	c, _ := newFakeClient(t, grants.respond)
	r := &mailboxPermissionGrantResource{client: c}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	empty := tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}
	iresp := resource.ImportStateResponse{State: empty}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "shared@contoso.com/a@contoso.com"}, &iresp)
	if iresp.Diagnostics.HasError() {
		t.Fatal(iresp.Diagnostics)
	}
	rresp := resource.ReadResponse{State: iresp.State}
	r.Read(ctx, resource.ReadRequest{State: iresp.State}, &rresp)
	var got mailboxPermissionGrantModel
	rresp.State.Get(ctx, &got)
	if !got.AccessRights.Equal(stringSetValue(ctx, []string{"FullAccess"})) || got.InheritanceType.ValueString() != "All" || !got.AutoMapping.IsNull() {
		t.Errorf("imported %+v", got)
	}
}

func TestRecipientPermissionCreate(t *testing.T) {
	fastPermissionConsistency(t)
	ctx := context.Background()
	grants := &fakeGrants{rights: map[string][]string{}}
	c, fake := newFakeClient(t, grants.respond)
	r := &recipientPermissionGrantResource{client: c}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	plan := tfsdk.Plan{Schema: sr.Schema}
	plan.Set(ctx, &recipientPermissionGrantModel{
		ID:           types.StringUnknown(),
		Identity:     types.StringValue("shared@contoso.com"),
		Trustee:      types.StringValue("a@contoso.com"),
		AccessRights: stringSetValue(ctx, []string{"SendAs"}),
	})
	cresp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &cresp)
	if cresp.Diagnostics.HasError() {
		t.Fatal(cresp.Diagnostics)
	}
	if call := fake.Calls()[0]; call.Cmdlet != "Add-RecipientPermission" || call.Params["Trustee"] != "a@contoso.com" {
		t.Errorf("first call = %+v", call)
	}
	var got recipientPermissionGrantModel
	cresp.State.Get(ctx, &got)
	if got.ID.ValueString() != "shared@contoso.com/a@contoso.com" {
		t.Errorf("id = %s", got.ID)
	}
}

func TestPermissionResourcesAreRegistered(t *testing.T) {
	ctx := context.Background()
	names := map[string]bool{}
	for _, f := range New("test")().Resources(ctx) {
		var md resource.MetadataResponse
		f().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "exo"}, &md)
		names[md.TypeName] = true
	}
	for _, n := range []string{"exo_mailbox_permission", "exo_recipient_permission"} {
		if !names[n] {
			t.Errorf("%s is not registered", n)
		}
	}
}
