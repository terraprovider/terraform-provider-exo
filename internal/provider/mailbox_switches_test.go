package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGetStateBoolReadsRecipientTypeDetails(t *testing.T) {
	cases := []struct {
		name  string
		obj   map[string]any
		field string
		want  bool
	}{
		{"shared mailbox", map[string]any{"RecipientTypeDetails": "SharedMailbox", "IsShared": true}, "Shared", true},
		{"user mailbox is not shared", map[string]any{"RecipientTypeDetails": "UserMailbox"}, "Shared", false},
		{"room mailbox", map[string]any{"RecipientTypeDetails": "RoomMailbox"}, "Room", true},
		{"room is not equipment", map[string]any{"RecipientTypeDetails": "RoomMailbox"}, "Equipment", false},
		{"equipment mailbox", map[string]any{"RecipientTypeDetails": "EquipmentMailbox"}, "Equipment", true},
		{"discovery mailbox", map[string]any{"RecipientTypeDetails": "DiscoveryMailbox"}, "Discovery", true},
		{"public folder mailbox", map[string]any{"RecipientTypeDetails": "PublicFolderMailbox"}, "PublicFolder", true},
		{"case-insensitive", map[string]any{"RecipientTypeDetails": "sharedmailbox"}, "Shared", true},
		{"unmapped switch", map[string]any{"RecipientTypeDetails": "SharedMailbox"}, "Migration", false},
		{"absent falls back to bool", map[string]any{"Shared": true}, "Shared", true},
	}
	for _, c := range cases {
		if got := getStateBool(c.obj, "RecipientTypeDetails", c.field); got != c.want {
			t.Errorf("%s: getStateBool = %v, want %v", c.name, got, c.want)
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
