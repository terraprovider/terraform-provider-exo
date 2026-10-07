package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-exoscc/adminapi"
)

// toJSON encodes a read-back object as a compact JSON string for a raw-json data
// source; decode it in configuration with jsondecode().
func toJSON(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// toStringSlice converts a types.Set of strings to []string.
func toStringSlice(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out
}

// stringSetValue builds a types.Set from a []string (empty, non-null when vals is nil).
func stringSetValue(ctx context.Context, vals []string) types.Set {
	if vals == nil {
		vals = []string{}
	}
	set, d := types.SetValueFrom(ctx, types.StringType, vals)
	if d.HasError() {
		return types.SetValueMust(types.StringType, nil)
	}
	return set
}

func firstObject(v []map[string]any) map[string]any {
	if len(v) == 0 {
		return nil
	}
	return v[0]
}

// getString returns the value under key as the string a string-typed
// attribute stores. The Admin API returns PowerShell booleans as JSON
// booleans, numbers as JSON numbers and multivalued parameters as arrays;
// treating any of those as "" makes Read never match the configured value
// and yields a perpetual diff (e.g. ExternalInOutlook.Enabled).
func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return coerceString(v)
}

func coerceString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s := coerceString(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// getStringSlice returns a multi-value property as a slice. Empty entries are
// dropped (the Admin API returns an empty list as [""], which would otherwise
// read back as a one-element set and diff against []), and a scalar string is
// treated as a one-element list.
func getStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	var arr []any
	switch t := v.(type) {
	case []any:
		arr = t
	case string:
		arr = []any{t}
	default:
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s := coerceString(e); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isNotFound reports whether the error indicates the object no longer exists
// (so the resource should be dropped from state on Read).
func isNotFound(err error) bool {
	var ae *adminapi.APIError
	if errors.As(err, &ae) {
		if ae.Status == 404 {
			return true
		}
		msg := strings.ToLower(ae.Message)
		return strings.Contains(msg, "couldn't be found") ||
			strings.Contains(msg, "not found") ||
			strings.Contains(msg, "managementobjectnotfound") ||
			strings.Contains(msg, "wasn't found")
	}
	return false
}

// getInt returns the value under key as the int64 an int-typed attribute
// stores. The Admin API decodes numbers as float64 (or json.Number) and
// occasionally renders them as strings; anything else reads as 0.
func getInt(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int64(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0
		}
		return n
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

// objectParam converts an untyped (System.Object) attribute's configured string
// for a write. A value that looks like JSON (starts with '[' or '{') is sent as
// the decoded structure; anything else as the plain string.
func objectParam(s string) any {
	t := strings.TrimSpace(s)
	if len(t) > 0 && (t[0] == '[' || t[0] == '{') {
		var v any
		if json.Unmarshal([]byte(t), &v) == nil {
			return v
		}
	}
	return s
}

// getObjectJSON reads an untyped (System.Object) attribute back as a string:
// strings as-is, any other value (number, bool, object, array) as compact JSON,
// so it round-trips against a literal or jsonencode() config.
func getObjectJSON(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
