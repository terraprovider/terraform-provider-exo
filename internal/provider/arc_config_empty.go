package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-exoscc/exo"
	"github.com/terraprovider/terraform-provider-exo/internal/clients"
)

const unconfiguredArcConfigID = "ArcConfig"

// arcConfigEmptyResource is exo_arc_config reading a tenant without ARC configuration as no trusted sealers.
type arcConfigEmptyResource struct{ *arcConfigResource }

// arcConfigEmptyDataSource is the exo_arc_config data source with the same empty read.
type arcConfigEmptyDataSource struct{ *arcConfigDataSource }

// withEmptyArcConfig replaces the generated exo_arc_config resource with arcConfigEmptyResource.
func withEmptyArcConfig(constructors []func() resource.Resource) []func() resource.Resource {
	out := make([]func() resource.Resource, len(constructors))
	for i, f := range constructors {
		out[i] = f
		if _, ok := f().(*arcConfigResource); ok {
			out[i] = func() resource.Resource { return arcConfigEmptyResource{&arcConfigResource{}} }
		}
	}
	return out
}

// withEmptyArcConfigDataSource replaces the generated exo_arc_config data source with arcConfigEmptyDataSource.
func withEmptyArcConfigDataSource(constructors []func() datasource.DataSource) []func() datasource.DataSource {
	out := make([]func() datasource.DataSource, len(constructors))
	for i, f := range constructors {
		out[i] = f
		if _, ok := f().(*arcConfigDataSource); ok {
			out[i] = func() datasource.DataSource { return arcConfigEmptyDataSource{&arcConfigDataSource{}} }
		}
	}
	return out
}

func (r arcConfigEmptyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.arcConfigResource.Create(ctx, req, resp)
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		return
	}
	var m arcConfigModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fillUnconfiguredArcConfig(ctx, &m)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r arcConfigEmptyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state arcConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, err := getArcConfig(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Get-ArcConfig failed", err.Error())
		return
	}
	if obj == nil {
		state.ArcTrustedSealers = stringSetValue(ctx, nil)
		fillUnconfiguredArcConfig(ctx, &state)
	} else {
		readArcConfig(ctx, obj, &state)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d arcConfigEmptyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data arcConfigModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, err := getArcConfig(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Get-ArcConfig failed", err.Error())
		return
	}
	if obj == nil {
		data.ArcTrustedSealers = stringSetValue(ctx, nil)
		fillUnconfiguredArcConfig(ctx, &data)
	} else {
		readArcConfig(ctx, obj, &data)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// getArcConfig returns the ArcConfig object, or nil when the tenant has none.
func getArcConfig(ctx context.Context, c *clients.Client) (map[string]any, error) {
	res, err := c.EXO.GetArcConfig(ctx, exo.GetArcConfigParams{})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return firstObject(res.Value), nil
}

// fillUnconfiguredArcConfig sets the attributes an unconfigured tenant has no value for.
func fillUnconfiguredArcConfig(ctx context.Context, m *arcConfigModel) {
	if m.ID.IsNull() || m.ID.IsUnknown() || m.ID.ValueString() == "" {
		m.ID = types.StringValue(unconfiguredArcConfigID)
	}
	if m.Identity.IsNull() || m.Identity.IsUnknown() || m.Identity.ValueString() == "" {
		m.Identity = types.StringValue(unconfiguredArcConfigID)
	}
	if m.ArcTrustedSealers.IsNull() || m.ArcTrustedSealers.IsUnknown() {
		m.ArcTrustedSealers = stringSetValue(ctx, nil)
	}
}
