package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// readDkimResource runs the resource Read against a fake Get-DkimSigningConfig
// that answers with objs, and reports the state left behind (nil: removed).
func readDkimResource(t *testing.T, objs []map[string]any, prior dkimSigningConfigModel) *dkimSigningConfigModel {
	t.Helper()
	ctx := context.Background()
	c, _ := newFakeClient(t, func(cmdlet string, _ map[string]any) []map[string]any {
		if cmdlet != "Get-DkimSigningConfig" {
			return nil
		}
		return objs
	})
	r := &dkimSigningConfigResource{client: c}
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
	if resp.State.Raw.IsNull() {
		return nil
	}
	var got dkimSigningConfigModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	return &got
}

func importedDkim(id string) dkimSigningConfigModel {
	return dkimSigningConfigModel{
		ID: types.StringValue(id), Identity: types.StringValue(id),
		AdminDisplayName: types.StringNull(), BodyCanonicalization: types.StringNull(),
		DomainName: types.StringNull(), Enabled: types.BoolNull(),
		HeaderCanonicalization: types.StringNull(), KeySize: types.Int64Null(),
		PublishTxtRecords: types.BoolNull(),
	}
}

// #25: Get-DkimSigningConfig answers an unknown identity with HTTP 200 and [{}].
func TestDkimSigningConfigReadEmptyObjectIsAbsent(t *testing.T) {
	if got := readDkimResource(t, []map[string]any{{}}, importedDkim("no-keys.example.com")); got != nil {
		t.Errorf("state kept for an empty object: %+v", *got)
	}
}

// #24: the domain is returned as Domain, the key size as Selector1KeySize, and
// PublishTxtRecords not at all.
func TestDkimSigningConfigReadAliases(t *testing.T) {
	obj := map[string]any{
		"Domain": "contoso.com", "Identity": "contoso.com", "Guid": "1bce8093-8101-4cc8-b5b5-d4d54e2a73cb",
		"Enabled": true, "Selector1KeySize": 2048, "Selector2KeySize": 2048,
		"HeaderCanonicalization": "Relaxed", "BodyCanonicalization": "Relaxed",
	}
	prior := importedDkim("contoso.com")
	prior.PublishTxtRecords = types.BoolValue(true)
	got := readDkimResource(t, []map[string]any{obj}, prior)
	if got == nil {
		t.Fatal("state removed for an existing config")
	}
	if got.DomainName.ValueString() != "contoso.com" {
		t.Errorf("domain_name = %s, want contoso.com", got.DomainName)
	}
	if got.KeySize.ValueInt64() != 2048 {
		t.Errorf("key_size = %s, want 2048", got.KeySize)
	}
	if !got.PublishTxtRecords.Equal(types.BoolValue(true)) {
		t.Errorf("publish_txt_records = %s, want the prior true", got.PublishTxtRecords)
	}
}
