package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func arcConfigResponder(sealers []any) func(string, map[string]any) []map[string]any {
	return func(cmdlet string, _ map[string]any) []map[string]any {
		if cmdlet != "Get-ArcConfig" || sealers == nil {
			return nil
		}
		return []map[string]any{{"Identity": "contoso\\ArcConfig", "Guid": "g-1", "ArcTrustedSealers": sealers}}
	}
}

func readArcConfigResource(t *testing.T, sealers []any, prior arcConfigModel) arcConfigModel {
	t.Helper()
	ctx := context.Background()
	c, _ := newFakeClient(t, arcConfigResponder(sealers))
	r := arcConfigEmptyResource{&arcConfigResource{client: c}}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	req := resource.ReadRequest{State: tfsdk.State{Schema: sr.Schema}}
	if d := req.State.Set(ctx, &prior); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: tfsdk.State{Schema: sr.Schema, Raw: req.State.Raw.Copy()}}
	r.Read(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got arcConfigModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	return got
}

func TestArcConfigResourceReadsUnconfiguredTenantAsEmpty(t *testing.T) {
	imported := arcConfigModel{ID: types.StringValue("default"), Identity: types.StringValue("default"), ArcTrustedSealers: types.SetNull(types.StringType)}
	got := readArcConfigResource(t, nil, imported)
	if !got.ArcTrustedSealers.Equal(stringSetValue(context.Background(), nil)) {
		t.Errorf("arc_trusted_sealers = %s, want []", got.ArcTrustedSealers)
	}
	if got.ID.ValueString() != "default" || got.Identity.ValueString() != "default" {
		t.Errorf("id/identity = %s/%s, want the import id", got.ID, got.Identity)
	}
}

func TestArcConfigResourceReadsConfiguredTenant(t *testing.T) {
	prior := arcConfigModel{ID: types.StringValue("default"), Identity: types.StringValue("default"), ArcTrustedSealers: types.SetNull(types.StringType)}
	got := readArcConfigResource(t, []any{"fabrikam.com"}, prior)
	want := stringSetValue(context.Background(), []string{"fabrikam.com"})
	if !got.ArcTrustedSealers.Equal(want) || got.ID.ValueString() != "g-1" {
		t.Errorf("got id %s sealers %s, want g-1 [fabrikam.com]", got.ID, got.ArcTrustedSealers)
	}
}

func TestArcConfigDataSourceReadsUnconfiguredTenantAsEmpty(t *testing.T) {
	ctx := context.Background()
	c, fake := newFakeClient(t, arcConfigResponder(nil))
	d := arcConfigEmptyDataSource{&arcConfigDataSource{client: c}}
	var sr datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sr)
	cfg := tfsdk.State{Schema: sr.Schema}
	if diags := cfg.Set(ctx, &arcConfigModel{ID: types.StringNull(), Identity: types.StringNull(), ArcTrustedSealers: types.SetNull(types.StringType)}); diags.HasError() {
		t.Fatal(diags)
	}
	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sr.Schema, Raw: cfg.Raw}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: sr.Schema}}
	d.Read(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got arcConfigModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if !got.ArcTrustedSealers.Equal(stringSetValue(ctx, nil)) || got.ID.ValueString() != unconfiguredArcConfigID {
		t.Errorf("got id %s sealers %s, want %s []", got.ID, got.ArcTrustedSealers, unconfiguredArcConfigID)
	}
	if n := len(fake.Calls()); n != 1 {
		t.Errorf("Get-ArcConfig called %d times, want 1 (no consistency retries)", n)
	}
}

func TestArcConfigIsRegisteredWithEmptyWrapper(t *testing.T) {
	ctx := context.Background()
	p := New("test")()
	var res, ds bool
	for _, f := range p.Resources(ctx) {
		if r, ok := f().(arcConfigEmptyResource); ok {
			res = true
			var _ resource.ResourceWithImportState = r
			var _ resource.ResourceWithModifyPlan = r
		}
	}
	for _, f := range p.DataSources(ctx) {
		if _, ok := f().(arcConfigEmptyDataSource); ok {
			ds = true
		}
	}
	if !res || !ds {
		t.Errorf("exo_arc_config wrapped: resource %v, data source %v", res, ds)
	}
}
