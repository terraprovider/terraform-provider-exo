package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/terraprovider/go-exoscc/spec"
	"github.com/terraprovider/tf-msadmin/genframework"
)

func sets(names ...string) []spec.ParamSet {
	out := make([]spec.ParamSet, 0, len(names))
	for _, n := range names {
		mandatory := n[len(n)-1] == '*'
		if mandatory {
			n = n[:len(n)-1]
		}
		out = append(out, spec.ParamSet{Name: n, Mandatory: mandatory})
	}
	return out
}

// newMailbox is a trimmed New-Mailbox with two type switches in their own parameter sets.
var newMailbox = spec.Cmdlet{Cmdlet: "New-Mailbox", Parameters: []spec.Param{
	{Name: "Name", Type: "System.String", ParameterSets: sets("__AllParameterSets*")},
	{Name: "Alias", Type: "System.String", ParameterSets: sets("__AllParameterSets")},
	{Name: "Shared", IsSwitch: true, ParameterSets: sets("Shared*")},
	{Name: "Room", IsSwitch: true, ParameterSets: sets("Room*", "EnableRoomMailboxAccount*")},
	{Name: "Password", Type: "System.Security.SecureString", ParameterSets: sets("User*", "Room", "Shared")},
	{Name: "Archive", IsSwitch: true, ParameterSets: sets("__AllParameterSets")},
	{Name: "Remote", IsSwitch: true, ParameterSets: sets("User*", "Room*")},
}}

func TestMandatoryInEverySet(t *testing.T) {
	cases := map[string]bool{"Name": true, "Alias": false, "Shared": false, "Room": false, "Password": false}
	for name, want := range cases {
		if got := mandatoryInEverySet(newMailbox, name); got != want {
			t.Errorf("mandatoryInEverySet(%s) = %v, want %v", name, got, want)
		}
	}
	single := spec.Cmdlet{Parameters: []spec.Param{
		{Name: "Name", ParameterSets: sets("__AllParameterSets*")},
		{Name: "Other", ParameterSets: sets("__AllParameterSets")},
	}}
	if !mandatoryInEverySet(single, "Name") || mandatoryInEverySet(single, "Other") {
		t.Error("a cmdlet without named sets should require only its mandatory parameters")
	}
}

func TestSwitchConflicts(t *testing.T) {
	attrs := []genframework.Attribute{
		{Field: "Shared", TFName: "shared"},
		{Field: "Room", TFName: "room"},
		{Field: "Archive", TFName: "archive"},
		{Field: "Remote", TFName: "remote"},
	}
	got := switchConflicts(newMailbox, attrs)
	want := [][2]string{{"remote", "shared"}, {"room", "shared"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("switchConflicts = %v, want %v", got, want)
	}
}

func TestGenValidators(t *testing.T) {
	src, err := genValidators(map[string][][2]string{"CASMailbox": {{"a", "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func (r *cASMailboxResource) ConfigValidators(", `{"a", "b"},`} {
		if !strings.Contains(string(src), want) {
			t.Errorf("generated validators lack %q:\n%s", want, src)
		}
	}
	if _, err := genValidators(nil); err != nil {
		t.Errorf("empty validators: %v", err)
	}
}

func TestRecipientTypeField(t *testing.T) {
	if got := recipientTypeField("Mailbox", "Shared", true); got != "RecipientTypeDetails" {
		t.Errorf("Mailbox Shared = %q", got)
	}
	for _, c := range []struct {
		noun, name string
		selector   bool
	}{{"Mailbox", "Migration", true}, {"Mailbox", "Shared", false}, {"MailUser", "Shared", true}} {
		if got := recipientTypeField(c.noun, c.name, c.selector); got != "" {
			t.Errorf("%s %s selector=%v = %q, want empty", c.noun, c.name, c.selector, got)
		}
	}
}

func TestWriteOnly(t *testing.T) {
	cases := []struct {
		p    spec.Param
		want bool
	}{
		{spec.Param{Name: "Password", Type: "securestring"}, true},
		{spec.Param{Name: "RemoteCredential", Type: "pscredential"}, true},
		{spec.Param{Name: "Credentials", Type: "System.Object", DeclaredType: "System.Management.Automation.PSCredential"}, true},
		{spec.Param{Name: "OAuthCode", Type: "System.Security.SecureString"}, true},
		{spec.Param{Name: "MinPasswordLength", Type: "System.Object"}, false},
		{spec.Param{Name: "AllowSimplePassword", Type: "bool"}, false},
		{spec.Param{Name: "AppSecretKeyVaultUrl", Type: "string"}, false},
	}
	for _, c := range cases {
		if got := writeOnly(c.p); got != c.want {
			t.Errorf("writeOnly(%s %s/%s) = %v, want %v", c.p.Name, c.p.Type, c.p.DeclaredType, got, c.want)
		}
	}
}
