package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestMailboxReadKeepsPassword: Get-Mailbox never returns the password, so a
// refresh must not overwrite the configured value with "".
func TestMailboxReadKeepsPassword(t *testing.T) {
	m := mailboxModel{Password: types.StringValue("s3cret"), RoomMailboxPassword: types.StringNull()}
	readMailbox(context.Background(), map[string]any{"Identity": "shared1", "RecipientTypeDetails": "UserMailbox"}, &m)
	if !m.Password.Equal(types.StringValue("s3cret")) {
		t.Errorf("password after read = %s, want the configured value", m.Password)
	}
	if !m.RoomMailboxPassword.IsNull() {
		t.Errorf("room_mailbox_password after read = %s, want null", m.RoomMailboxPassword)
	}
}

// TestMailboxUnsetPasswordIsKnownAfterApply: an unconfigured Optional+Computed
// password is unknown in the plan and nothing reads it back; Create must not
// store an unknown value.
func TestMailboxUnsetPasswordIsKnownAfterApply(t *testing.T) {
	r := &mailboxResource{}
	cfg := mailboxModel{Password: types.StringUnknown(), RoomMailboxPassword: types.StringValue("room")}
	read := cfg
	r.reconcileState(&cfg, &read)
	if !read.Password.IsNull() {
		t.Errorf("unset password after apply = %s, want null", read.Password)
	}
	if !read.RoomMailboxPassword.Equal(types.StringValue("room")) {
		t.Errorf("room_mailbox_password after apply = %s, want the configured value", read.RoomMailboxPassword)
	}
}

func TestReplaceIfWriteOnlyRecorded(t *testing.T) {
	cases := map[string]struct {
		state types.String
		want  bool
	}{
		"imported (null state) only records": {types.StringNull(), false},
		"empty legacy state only records":    {types.StringValue(""), false},
		"changed recorded value replaces":    {types.StringValue("old"), true},
	}
	for name, c := range cases {
		req := planmodifier.StringRequest{StateValue: c.state, PlanValue: types.StringValue("new")}
		var resp stringplanmodifier.RequiresReplaceIfFuncResponse
		replaceIfWriteOnlyRecorded(context.Background(), req, &resp)
		if resp.RequiresReplace != c.want {
			t.Errorf("%s: RequiresReplace = %v, want %v", name, resp.RequiresReplace, c.want)
		}
	}
}
