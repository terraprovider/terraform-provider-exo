package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestReadMailboxTypeFromRecipientTypeDetails(t *testing.T) {
	switches := func(m mailboxModel) map[string]bool {
		return map[string]bool{
			"Shared": m.Shared.ValueBool(), "Room": m.Room.ValueBool(), "Equipment": m.Equipment.ValueBool(),
			"Discovery": m.Discovery.ValueBool(), "PublicFolder": m.PublicFolder.ValueBool(),
		}
	}
	cases := []struct {
		name     string
		obj      map[string]any
		switchOn string // "" = none
		typ      string
	}{
		{"shared mailbox", map[string]any{"RecipientTypeDetails": "SharedMailbox", "IsShared": true}, "Shared", "Shared"},
		{"user mailbox", map[string]any{"RecipientTypeDetails": "UserMailbox"}, "", "Regular"},
		{"room mailbox", map[string]any{"RecipientTypeDetails": "RoomMailbox"}, "Room", "Room"},
		{"equipment mailbox", map[string]any{"RecipientTypeDetails": "EquipmentMailbox"}, "Equipment", "Equipment"},
		{"discovery mailbox has no -Type", map[string]any{"RecipientTypeDetails": "DiscoveryMailbox"}, "Discovery", ""},
		{"public folder mailbox has no -Type", map[string]any{"RecipientTypeDetails": "PublicFolderMailbox"}, "PublicFolder", ""},
		{"case-insensitive", map[string]any{"RecipientTypeDetails": "sharedmailbox"}, "Shared", "Shared"},
		{"absent falls back to the bool property", map[string]any{"Shared": true}, "Shared", ""},
	}
	for _, c := range cases {
		var m mailboxModel
		readMailbox(context.Background(), c.obj, &m)
		for name, on := range switches(m) {
			if on != (name == c.switchOn) {
				t.Errorf("%s: %s = %v", c.name, name, on)
			}
		}
		if got := m.Type.ValueString(); got != c.typ {
			t.Errorf("%s: type = %q, want %q", c.name, got, c.typ)
		}
	}
}

func TestMailboxTypeSwitchChangeFailsThePlan(t *testing.T) {
	ctx := context.Background()
	guard := failOnChangeBool("use type")
	cases := []struct {
		name        string
		state, plan types.Bool
		wantError   bool
	}{
		{"configured change", types.BoolValue(true), types.BoolValue(false), true},
		{"unconfigured (unknown before UseStateForUnknown)", types.BoolValue(true), types.BoolUnknown(), false},
		{"no prior value", types.BoolNull(), types.BoolValue(true), false},
	}
	for _, c := range cases {
		var resp boolplanmodifier.RequiresReplaceIfFuncResponse
		guard(ctx, planmodifier.BoolRequest{Path: path.Root("shared"), StateValue: c.state, PlanValue: c.plan}, &resp)
		if resp.Diagnostics.HasError() != c.wantError {
			t.Errorf("%s: error = %v, want %v", c.name, resp.Diagnostics, c.wantError)
		}
		if resp.RequiresReplace {
			t.Errorf("%s: asked for replacement", c.name)
		}
	}
}

func TestConflictingSwitchesRejectsTwoTrue(t *testing.T) {
	ctx := context.Background()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"shared": schema.BoolAttribute{Optional: true},
		"room":   schema.BoolAttribute{Optional: true},
	}}
	v := conflictingSwitches{{"room", "shared"}}
	cases := []struct {
		name         string
		shared, room types.Bool
		wantErr      bool
	}{
		{"both true", types.BoolValue(true), types.BoolValue(true), true},
		{"one true", types.BoolValue(true), types.BoolValue(false), false},
		{"one unset", types.BoolValue(true), types.BoolNull(), false},
		{"unknown", types.BoolValue(true), types.BoolUnknown(), false},
		{"none", types.BoolNull(), types.BoolNull(), false},
	}
	for _, c := range cases {
		obj, d := types.ObjectValue(
			map[string]attr.Type{"shared": types.BoolType, "room": types.BoolType},
			map[string]attr.Value{"shared": c.shared, "room": c.room},
		)
		if d.HasError() {
			t.Fatalf("%s: %v", c.name, d)
		}
		raw, err := obj.ToTerraformValue(ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}
		var resp resource.ValidateConfigResponse
		v.ValidateResource(ctx, req, &resp)
		if got := resp.Diagnostics.HasError(); got != c.wantErr {
			t.Errorf("%s: error = %v, want %v (%v)", c.name, got, c.wantErr, resp.Diagnostics)
		}
	}
}

func TestMailboxTypeSwitchesAreOptionalAndExclusive(t *testing.T) {
	ctx := context.Background()
	r := NewMailboxResource()
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	for _, name := range []string{"discovery", "equipment", "migration", "public_folder", "room", "shared", "password"} {
		a, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("exo_mailbox has no %s attribute", name)
		}
		if a.IsRequired() || !a.IsOptional() {
			t.Errorf("%s: want Optional, got Required=%v Optional=%v", name, a.IsRequired(), a.IsOptional())
		}
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name: want Required")
	}
	cv, ok := r.(resource.ResourceWithConfigValidators)
	if !ok {
		t.Fatal("exo_mailbox has no config validators")
	}
	found := false
	for _, v := range cv.ConfigValidators(ctx) {
		if pairs, ok := v.(conflictingSwitches); ok {
			for _, p := range pairs {
				if p == [2]string{"room", "shared"} {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("room and shared are not mutually exclusive")
	}
}

// TestMailboxTypeChangeReplansSwitches: converting with type leaves the
// unconfigured switches unknown, so the apply reads them back instead of
// keeping the old state value (shared = true on what is now a room mailbox).
func TestMailboxTypeChangeReplansSwitches(t *testing.T) {
	ctx := context.Background()
	var sr resource.SchemaResponse
	(&mailboxResource{}).Schema(ctx, resource.SchemaRequest{}, &sr)
	newState := func(typ string, shared bool) tfsdk.State {
		st := tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}
		for p, v := range map[string]attr.Value{"name": types.StringValue("mbx"), "type": types.StringValue(typ), "shared": types.BoolValue(shared)} {
			if d := st.SetAttribute(ctx, path.Root(p), v); d.HasError() {
				t.Fatal(d)
			}
		}
		return st
	}
	state := newState("Shared", true)
	cases := []struct {
		name        string
		plannedType string
		wantUnknown bool
	}{
		{"type changed", "Room", true},
		{"type unchanged", "Shared", false},
		{"type unconfigured", "", false},
		{"type configured but unknown", "?", true},
	}
	for _, c := range cases {
		plan := newState("Shared", true)
		cfgState := tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}
		var typ types.String
		switch c.plannedType {
		case "":
			typ = types.StringNull()
		case "?":
			typ = types.StringUnknown()
		default:
			typ = types.StringValue(c.plannedType)
		}
		if c.plannedType != "" {
			plan = newState("Shared", true)
			if d := plan.SetAttribute(ctx, path.Root("type"), typ); d.HasError() {
				t.Fatal(d)
			}
		} else if d := plan.SetAttribute(ctx, path.Root("type"), types.StringUnknown()); d.HasError() {
			t.Fatal(d) // unconfigured Optional+Computed: the framework plans it unknown
		}
		if d := cfgState.SetAttribute(ctx, path.Root("type"), typ); d.HasError() {
			t.Fatal(d)
		}
		cfg := tfsdk.Config{Schema: sr.Schema, Raw: cfgState.Raw}
		req := planmodifier.BoolRequest{
			Path: path.Root("shared"), ConfigValue: types.BoolNull(), Config: cfg,
			State: state, Plan: tfsdk.Plan{Schema: sr.Schema, Raw: plan.Raw},
			StateValue: types.BoolValue(true), PlanValue: types.BoolValue(true),
		}
		resp := planmodifier.BoolResponse{PlanValue: req.PlanValue}
		unknownIfChangedBool("type").PlanModifyBool(ctx, req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", c.name, resp.Diagnostics)
		}
		if resp.PlanValue.IsUnknown() != c.wantUnknown {
			t.Errorf("%s: shared planned %s, want unknown=%v", c.name, resp.PlanValue, c.wantUnknown)
		}
	}
}
