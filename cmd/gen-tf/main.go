// Command gen-tf generates terraform-plugin-framework resources for the
// Exchange Online provider from the go-exoscc cmdlet catalog. It is the
// EXO-specific frontend to the reusable tf-msadmin/genframework engine: it turns
// CRUD-complete nouns (New+Get+Set+Remove) into normalized genframework.Resource
// values and writes the emitted files into internal/provider.
//
//	go run ./cmd/gen-tf                 # generate every CRUD-complete noun
//	go run ./cmd/gen-tf -noun RoleGroup # just one (for iterating)
//	go run ./cmd/gen-tf -out /tmp/x     # write elsewhere
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/terraprovider/go-exoscc/spec"
	"github.com/terraprovider/tf-msadmin/genframework"
)

func main() {
	var only, out string
	flag.StringVar(&only, "noun", "", "generate only this noun (default: all CRUD-complete)")
	flag.StringVar(&out, "out", "internal/provider", "output directory")
	flag.Parse()

	cat, err := spec.EXO()
	check(err)

	cfg := genframework.Config{
		Package:        "provider",
		ClientsImport:  "github.com/terraprovider/terraform-provider-exo/internal/clients",
		ClientField:    "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo",
		BindingsPkg:    "exo",
	}

	byNoun := cat.ByNoun()
	var resources []genframework.Resource
	var skipped []string
	for _, noun := range cat.CRUDComplete() {
		if only != "" && noun != only {
			continue
		}
		r, ok, reason := buildResource(noun, byNoun[noun], byNoun)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", noun, reason))
			continue
		}
		resources = append(resources, r)
	}

	// Config resources: Get+Set nouns with no New (org singletons + per-object
	// configs like Set-AdminAuditLogConfig / Set-CASMailbox).
	var configCount int
	for _, noun := range configNouns(byNoun) {
		if only != "" && noun != only {
			continue
		}
		r, ok, reason := buildConfigResource(noun, byNoun[noun])
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s config (%s)", noun, reason))
			continue
		}
		resources = append(resources, r)
		configCount++
	}
	fmt.Printf("(%d config resources)\n", configCount)

	// Read-only lookup objects: Get-only nouns (with -Identity) that are real
	// objects (not reports/diagnostics/sub-collections) become json data sources.
	var jsonDSCount int
	for _, noun := range jsonObjectNouns(byNoun) {
		if only != "" && noun != only {
			continue
		}
		resources = append(resources, buildJSONDataSource(noun, byNoun[noun]))
		jsonDSCount++
	}
	fmt.Printf("(%d json data sources)\n", jsonDSCount)

	files, err := genframework.Generate(cfg, resources)
	check(err)

	check(os.MkdirAll(out, 0o755))
	for _, f := range files {
		check(os.WriteFile(filepath.Join(out, f.Name), f.Content, 0o644))
	}
	fmt.Printf("generated %d resource(s), %d file(s) -> %s\n", len(resources), len(files), out)
	if len(skipped) > 0 {
		sort.Strings(skipped)
		fmt.Printf("skipped %d noun(s):\n  %s\n", len(skipped), strings.Join(skipped, "\n  "))
	}
}

// buildResource maps a noun's CRUD cmdlets into a genframework.Resource.
func buildResource(noun string, verbs map[string]spec.Cmdlet, byNoun map[string]map[string]spec.Cmdlet) (genframework.Resource, bool, string) {
	newCmd := verbs["New"]
	getCmd := verbs["Get"]
	setCmd := verbs["Set"]
	removeCmd := verbs["Remove"]

	// Per-operation lookup key. Get and Remove must be able to target the object;
	// Set may take no key (an org-wide setter such as Set-AvailabilityConfig).
	readKey := identityFieldFor(getCmd, noun)
	removeKey := identityFieldFor(removeCmd, noun)
	if readKey == "" || removeKey == "" {
		return genframework.Resource{}, false, "no lookup key on Get/Remove"
	}
	setKey := identityFieldFor(setCmd, noun)

	inNew := paramSet(newCmd)
	inSet := paramSet(setCmd)

	// Union of New and Set params, in a stable order.
	names := unionKeys(inNew, inSet)

	idExclude := map[string]bool{"Identity": true, readKey: true, removeKey: true}
	if setKey != "" {
		idExclude[setKey] = true
	}

	// A members sub-collection (Get-/Update-<Noun>Member) is handled separately;
	// exclude the main noun's Members param so it isn't also a plain attribute.
	members := memberCollection(noun, byNoun)
	if members != nil {
		idExclude["Members"] = true
		idExclude["Member"] = true
	}

	var attrs []genframework.Attribute
	for _, name := range names {
		if skipParam(name) || idExclude[name] {
			continue
		}
		p := firstParam(name, newCmd, setCmd)
		at, ok := attrType(p)
		if !ok {
			continue // unmappable (any/int/complex)
		}
		field := exportName(name)
		_, inC := inNew[name]
		_, inU := inSet[name]
		required := inC && newCmd.Parameters != nil && mandatoryIn(newCmd, name)
		replace := required || (inC && !inU)
		if replace {
			inU = false // replace-only attributes are never updated in place
		}
		attrs = append(attrs, genframework.Attribute{
			TFName:      tfName(field),
			Field:       field,
			APIName:     field,
			Type:        at,
			Required:    required,
			Computed:    !required,
			Sensitive:   sensitive(name),
			Replace:     replace,
			Description: describe(name, p),
			InCreate:    inC,
			InUpdate:    inU,
			Object:      goType(p) == "any",
		})
	}
	if !hasCreateAttr(attrs) {
		return genframework.Resource{}, false, "no mappable create parameters"
	}

	identityRead := ""
	if readKey != "Identity" {
		identityRead = exportName(readKey)
	}
	return genframework.Resource{
		Noun:              noun,
		TFName:            pascalToSnake(noun),
		Description:       fmt.Sprintf("Manages the %s object via %s / %s / %s / %s.", noun, newCmd.Cmdlet, getCmd.Cmdlet, setCmd.Cmdlet, removeCmd.Cmdlet),
		IdentityReadField: identityRead,
		Attributes:        attrs,
		Create:            op(newCmd, ""),
		Read:              op(getCmd, exportName(readKey)),
		Update:            op(setCmd, exportNameOrEmpty(setKey)),
		Delete:            op(removeCmd, exportName(removeKey)),
		Members:           members,
	}, true, ""
}

// memberCollection detects a <Noun>Member companion family with a Get (to read)
// and an Update ... -Members (to replace the whole set), e.g. RoleGroupMember /
// DistributionGroupMember.
func memberCollection(noun string, byNoun map[string]map[string]spec.Cmdlet) *genframework.MemberCollection {
	mv, ok := byNoun[noun+"Member"]
	if !ok {
		return nil
	}
	get, hasGet := mv["Get"]
	upd, hasUpd := mv["Update"]
	if !hasGet || !hasUpd {
		return nil
	}
	if !paramSet(get)["Identity"] || !paramSet(upd)["Identity"] || !paramSet(upd)["Members"] {
		return nil
	}
	return &genframework.MemberCollection{
		TFName:        "members",
		Field:         "Members",
		Description:   fmt.Sprintf("Members of the %s, managed via %s.", noun, upd.Cmdlet),
		ReadMethod:    goName(get.Cmdlet),
		ReadParams:    goName(get.Cmdlet) + "Params",
		UpdateMethod:  goName(upd.Cmdlet),
		UpdateParams:  goName(upd.Cmdlet) + "Params",
		IdentityField: "Identity",
		MembersField:  "Members",
		ReadKeys:      []string{"PrimarySmtpAddress", "Name", "Identity"},
	}
}

func op(c spec.Cmdlet, identityField string) genframework.Op {
	m := goName(c.Cmdlet)
	return genframework.Op{Method: m, Params: m + "Params", IdentityField: identityField}
}

// jsonObjectNouns are Get-only nouns (Get, no Set/New) that take an -Identity and
// are real objects (not reports/diagnostics/sub-collections): read-only lookups.
func jsonObjectNouns(byNoun map[string]map[string]spec.Cmdlet) []string {
	var out []string
	for noun, v := range byNoun {
		getc, hasGet := v["Get"]
		_, hasSet := v["Set"]
		_, hasNew := v["New"]
		if !hasGet || hasSet || hasNew {
			continue
		}
		if !paramSet(getc)["Identity"] {
			continue
		}
		if !isLookupObject(noun) {
			continue
		}
		out = append(out, noun)
	}
	sort.Strings(out)
	return out
}

// isLookupObject excludes report/aggregate, diagnostic/log and sub-collection
// nouns, which are not "look up one object by identity" data sources.
func isLookupObject(n string) bool {
	for _, p := range []string{"Report", "Snapshot", "Statistics", "Aggregate", "Detection", "Summary", "Recommendation", "Impact", "Diagnostic", "CrawlState", "Analysis"} {
		if strings.Contains(n, p) {
			return false
		}
	}
	if strings.Contains(n, "Member") {
		return false
	}
	for _, s := range []string{"Status", "Log", "Logs", "Links"} {
		if strings.HasSuffix(n, s) {
			return false
		}
	}
	return true
}

// buildJSONDataSource builds a raw-json data source for a read-only lookup noun.
func buildJSONDataSource(noun string, verbs map[string]spec.Cmdlet) genframework.Resource {
	getc := verbs["Get"]
	return genframework.Resource{
		Noun:           noun,
		TFName:         pascalToSnake(noun),
		Description:    fmt.Sprintf("Looks up a %s object via %s and exposes it as a json attribute.", noun, getc.Cmdlet),
		Read:           genframework.Op{Method: goName(getc.Cmdlet), Params: goName(getc.Cmdlet) + "Params", IdentityField: "Identity"},
		DataSourceOnly: true,
		RawJSON:        true,
	}
}

// configNouns are the Get+Set nouns without a New verb — config objects managed
// in place rather than created/destroyed.
func configNouns(byNoun map[string]map[string]spec.Cmdlet) []string {
	var out []string
	for noun, v := range byNoun {
		_, hasNew := v["New"]
		_, hasGet := v["Get"]
		_, hasSet := v["Set"]
		if hasGet && hasSet && !hasNew {
			out = append(out, noun)
		}
	}
	sort.Strings(out)
	return out
}

// buildConfigResource maps a Get+Set noun into a config genframework.Resource.
func buildConfigResource(noun string, verbs map[string]spec.Cmdlet) (genframework.Resource, bool, string) {
	getc, hasGet := verbs["Get"]
	setc, hasSet := verbs["Set"]
	if !hasGet || !hasSet {
		return genframework.Resource{}, false, "not Get+Set"
	}
	// No Identity on Get => an org-wide singleton; otherwise a per-object config.
	singleton := !paramSet(getc)["Identity"]

	var names []string
	for n := range paramSet(setc) {
		names = append(names, n)
	}
	sort.Strings(names)

	var attrs []genframework.Attribute
	for _, name := range names {
		if skipParam(name) || name == "Identity" {
			continue
		}
		p := firstParam(name, setc)
		at, ok := attrType(p)
		if !ok {
			continue
		}
		field := exportName(name)
		attrs = append(attrs, genframework.Attribute{
			TFName:      tfName(field),
			Field:       field,
			APIName:     field,
			Type:        at,
			Computed:    true, // settings: Optional + Computed
			Sensitive:   sensitive(name),
			Description: describe(name, p),
			InCreate:    true,
			InUpdate:    true,
			Object:      goType(p) == "any",
		})
	}
	if len(attrs) == 0 {
		return genframework.Resource{}, false, "no mappable settings"
	}

	readID := ""
	if paramSet(getc)["Identity"] {
		readID = "Identity"
	}
	updID := ""
	if !singleton && paramSet(setc)["Identity"] {
		updID = "Identity"
	}
	return genframework.Resource{
		Noun:        noun,
		TFName:      pascalToSnake(noun),
		Description: fmt.Sprintf("Manages the %s configuration via %s.", noun, setc.Cmdlet),
		Attributes:  attrs,
		Read:        genframework.Op{Method: goName(getc.Cmdlet), Params: goName(getc.Cmdlet) + "Params", IdentityField: readID},
		Update:      genframework.Op{Method: goName(setc.Cmdlet), Params: goName(setc.Cmdlet) + "Params", IdentityField: updID},
		Config:      true,
		Singleton:   singleton,
	}, true, ""
}

func exportNameOrEmpty(name string) string {
	if name == "" {
		return ""
	}
	return exportName(name)
}

// identityFieldFor picks a cmdlet's lookup-key parameter: prefer "Identity",
// then a <Noun>Id parameter, then a single unambiguous *Id parameter. Returns ""
// when the cmdlet has no usable key.
func identityFieldFor(c spec.Cmdlet, noun string) string {
	if paramSet(c)["Identity"] {
		return "Identity"
	}
	want := strings.ToLower(noun) + "id"
	var ids []string
	for _, p := range c.Parameters {
		ln := strings.ToLower(p.Name)
		if ln == want {
			return p.Name
		}
		if strings.HasSuffix(ln, "id") && !skipParam(p.Name) {
			ids = append(ids, p.Name)
		}
	}
	if len(ids) == 1 {
		return ids[0]
	}
	return ""
}

func attrType(p spec.Param) (genframework.AttrType, bool) {
	switch goType(p) {
	case "bool":
		return genframework.TypeBool, true
	case "[]string":
		return genframework.TypeStringSet, true
	case "string", "any":
		// "any" is a PowerShell System.Object — most are identity/string-valued
		// (an account, a role name, a GUID). Expose them as strings (best-effort);
		// complex objects simply read back empty and can be left unset.
		return genframework.TypeString, true
	default: // int
		return 0, false
	}
}

func hasCreateAttr(attrs []genframework.Attribute) bool {
	for _, a := range attrs {
		if a.InCreate {
			return true
		}
	}
	return false
}

// operational / plumbing switches that are cmdlet controls, not resource state.
var skipParams = map[string]bool{
	"Force": true, "Confirm": true, "WhatIf": true, "Verbose": true, "Debug": true,
	"BypassSecurityGroupManagerCheck": true, "ProgressAction": true,
	"ErrorAction": true, "WarningAction": true, "InformationAction": true,
	"ErrorVariable": true, "WarningVariable": true, "InformationVariable": true,
	"OutVariable": true, "OutBuffer": true, "PipelineVariable": true,
	"DomainController": true, "IgnoreDefaultScope": true, "ResultSize": true,
}

func skipParam(name string) bool { return skipParams[name] }

func sensitive(name string) bool {
	l := strings.ToLower(name)
	return strings.Contains(l, "password") || strings.Contains(l, "secret") || strings.Contains(l, "credential")
}

func describe(name string, p spec.Param) string {
	d := fmt.Sprintf("Maps to the -%s parameter.", name)
	if len(p.ValidateSet) > 0 {
		d += " Allowed values: " + strings.Join(p.ValidateSet, ", ") + "."
	}
	return d
}

// ---- catalog helpers ----

func paramSet(c spec.Cmdlet) map[string]bool {
	m := map[string]bool{}
	for _, p := range c.Parameters {
		m[p.Name] = true
	}
	return m
}

func firstParam(name string, cmds ...spec.Cmdlet) spec.Param {
	for _, c := range cmds {
		for _, p := range c.Parameters {
			if p.Name == name {
				return p
			}
		}
	}
	return spec.Param{Name: name}
}

func mandatoryIn(c spec.Cmdlet, name string) bool {
	for _, p := range c.Parameters {
		if p.Name == name {
			return p.Mandatory()
		}
	}
	return false
}

func unionKeys(a, b map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for k := range a {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ---- name/type mapping (mirrors go-exoscc/cmd/gen-go) ----

func goType(p spec.Param) string {
	if p.IsSwitch {
		return "bool"
	}
	t := strings.ToLower(p.Type)
	switch {
	case strings.HasSuffix(t, "[]"):
		return "[]string"
	case t == "string" || strings.HasSuffix(t, ".string"):
		return "string"
	case t == "bool" || strings.HasSuffix(t, ".boolean"):
		return "bool"
	case t == "int" || strings.HasSuffix(t, ".int32") || strings.HasSuffix(t, ".int64"):
		return "int"
	case strings.HasSuffix(t, ".guid"):
		return "string"
	default:
		return "any"
	}
}

func goName(cmdlet string) string {
	parts := strings.FieldsFunc(cmdlet, func(r rune) bool { return r == '-' || r == '_' })
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(exportName(p))
	}
	return sb.String()
}

func exportName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return "X"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "N" + s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// reservedNames are attribute names a generated resource may not reuse: the
// Terraform root reserved names, plus "id"/"identity" which every resource
// already declares as computed attributes.
var reservedNames = map[string]bool{
	"alias": true, "count": true, "depends_on": true, "for_each": true,
	"lifecycle": true, "provider": true, "provisioner": true, "connection": true,
	"id": true, "identity": true,
}

// tfName is the snake_case attribute name for a field, with a trailing
// underscore appended when it would collide with a reserved Terraform name (the
// Go field / API name is unchanged, so the -Parameter mapping is preserved).
func tfName(field string) string {
	n := pascalToSnake(field)
	if reservedNames[n] {
		n += "_"
	}
	return n
}

// pascalToSnake turns "DisplayName" into "display_name", keeping runs of
// uppercase letters together ("OWAEnabled" -> "owa_enabled").
func pascalToSnake(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && i > 0 {
			prev := runes[i-1]
			prevLower := prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if prevLower || nextLower {
				b.WriteByte('_')
			}
		}
		if isUpper {
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-tf:", err)
		os.Exit(1)
	}
}
