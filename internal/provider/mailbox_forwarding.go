package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// mailboxForwardingResource is exo_mailbox with forwarding_smtp_address stored without the smtp: prefix Get-Mailbox adds.
type mailboxForwardingResource struct{ *mailboxResource }

// newMailboxForwardingResource wraps the generated exo_mailbox resource.
func newMailboxForwardingResource() resource.Resource {
	return mailboxForwardingResource{NewMailboxResource().(*mailboxResource)}
}

// overrideResources replaces generated constructors with their hand-written wrappers.
func overrideResources(constructors []func() resource.Resource) []func() resource.Resource {
	out := make([]func() resource.Resource, len(constructors))
	for i, f := range constructors {
		out[i] = f
		if _, ok := f().(*mailboxResource); ok {
			out[i] = newMailboxForwardingResource
		}
	}
	return out
}

var forwardingSmtpAddress = path.Root("forwarding_smtp_address")

func (r mailboxForwardingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.mailboxResource.Create(ctx, req, resp)
	keepForwardingAddress(ctx, req.Plan.GetAttribute, &resp.State, &resp.Diagnostics)
}

func (r mailboxForwardingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.mailboxResource.Read(ctx, req, resp)
	keepForwardingAddress(ctx, req.State.GetAttribute, &resp.State, &resp.Diagnostics)
}

func (r mailboxForwardingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.mailboxResource.Update(ctx, req, resp)
	keepForwardingAddress(ctx, req.Plan.GetAttribute, &resp.State, &resp.Diagnostics)
}

// keepForwardingAddress rewrites the read-back forwarding_smtp_address in state to keepProxyAddress(prior, read).
func keepForwardingAddress(ctx context.Context, prior func(context.Context, path.Path, any) diag.Diagnostics, state *tfsdk.State, diags *diag.Diagnostics) {
	if diags.HasError() || state.Raw.IsNull() {
		return
	}
	var was, read types.String
	diags.Append(prior(ctx, forwardingSmtpAddress, &was)...)
	diags.Append(state.GetAttribute(ctx, forwardingSmtpAddress, &read)...)
	if diags.HasError() {
		return
	}
	diags.Append(state.SetAttribute(ctx, forwardingSmtpAddress, keepProxyAddress(was, read))...)
}

// keepProxyAddress returns prior when it names the same address as read, otherwise read without its smtp: prefix.
func keepProxyAddress(prior, read types.String) types.String {
	if read.IsNull() || read.IsUnknown() {
		return read
	}
	addr := stripSMTPPrefix(read.ValueString())
	if !prior.IsNull() && !prior.IsUnknown() && strings.EqualFold(stripSMTPPrefix(prior.ValueString()), addr) {
		return prior
	}
	return types.StringValue(addr)
}

func stripSMTPPrefix(s string) string {
	if len(s) >= len("smtp:") && strings.EqualFold(s[:len("smtp:")], "smtp:") {
		return s[len("smtp:"):]
	}
	return s
}
