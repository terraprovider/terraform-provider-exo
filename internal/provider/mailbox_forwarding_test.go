package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestKeepProxyAddress(t *testing.T) {
	cases := []struct {
		name        string
		prior, read types.String
		want        types.String
	}{
		{"import strips the prefix", types.StringNull(), types.StringValue("smtp:a@contoso.com"), types.StringValue("a@contoso.com")},
		{"unknown prior strips the prefix", types.StringUnknown(), types.StringValue("smtp:a@contoso.com"), types.StringValue("a@contoso.com")},
		{"bare config stays bare", types.StringValue("a@contoso.com"), types.StringValue("smtp:a@contoso.com"), types.StringValue("a@contoso.com")},
		{"prefixed state stays prefixed", types.StringValue("smtp:a@contoso.com"), types.StringValue("smtp:a@contoso.com"), types.StringValue("smtp:a@contoso.com")},
		{"uppercase prefix", types.StringValue("SMTP:a@contoso.com"), types.StringValue("smtp:a@contoso.com"), types.StringValue("SMTP:a@contoso.com")},
		{"case-insensitive address", types.StringValue("A@Contoso.com"), types.StringValue("smtp:a@contoso.com"), types.StringValue("A@Contoso.com")},
		{"changed outside Terraform", types.StringValue("a@contoso.com"), types.StringValue("smtp:b@contoso.com"), types.StringValue("b@contoso.com")},
		{"forwarding removed", types.StringValue("a@contoso.com"), types.StringValue(""), types.StringValue("")},
		{"no prefix on read", types.StringNull(), types.StringValue("a@contoso.com"), types.StringValue("a@contoso.com")},
	}
	for _, c := range cases {
		if got := keepProxyAddress(c.prior, c.read); !got.Equal(c.want) {
			t.Errorf("%s: keepProxyAddress(%s, %s) = %s, want %s", c.name, c.prior, c.read, got, c.want)
		}
	}
}

func TestMailboxIsRegisteredWithForwardingWrapper(t *testing.T) {
	ctx := context.Background()
	var found bool
	for _, f := range New("test")().Resources(ctx) {
		r := f()
		var md resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "exo"}, &md)
		if md.TypeName != "exo_mailbox" {
			continue
		}
		found = true
		if _, ok := r.(mailboxForwardingResource); !ok {
			t.Errorf("exo_mailbox is %T, want mailboxForwardingResource", r)
		}
		if _, ok := r.(resource.ResourceWithImportState); !ok {
			t.Error("the wrapper lost ImportState")
		}
		if _, ok := r.(resource.ResourceWithConfigure); !ok {
			t.Error("the wrapper lost Configure")
		}
	}
	if !found {
		t.Fatal("exo_mailbox is not registered")
	}
}
