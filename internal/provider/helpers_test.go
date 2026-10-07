package provider

import (
	"reflect"
	"testing"
)

func TestGetStringCoercesAdminAPIValues(t *testing.T) {
	obj := map[string]any{
		"Str":   "x",
		"Bool":  true,
		"Num":   float64(7),
		"List":  []any{"a", "b"},
		"Empty": []any{},
		"Nil":   nil,
	}
	cases := map[string]string{"Str": "x", "Bool": "true", "Num": "7", "List": "a,b", "Empty": "", "Nil": "", "Missing": ""}
	for k, want := range cases {
		if got := getString(obj, k); got != want {
			t.Errorf("getString(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestGetIntCoercesAdminAPIValues(t *testing.T) {
	obj := map[string]any{
		"Num":  float64(25),
		"Str":  "7",
		"Bad":  "abc",
		"List": []any{1},
		"Nil":  nil,
	}
	cases := map[string]int64{"Num": 25, "Str": 7, "Bad": 0, "List": 0, "Nil": 0, "Missing": 0}
	for k, want := range cases {
		if got := getInt(obj, k); got != want {
			t.Errorf("getInt(%q) = %d, want %d", k, got, want)
		}
	}
}

func TestGetStringSliceDropsEmptyEntries(t *testing.T) {
	obj := map[string]any{
		"List":      []any{"a", "", "b"},
		"EmptyElem": []any{""},
		"Empty":     []any{},
		"Scalar":    "x",
		"EmptyStr":  "",
		"Nil":       nil,
	}
	cases := map[string][]string{"List": {"a", "b"}, "EmptyElem": {}, "Empty": {}, "Scalar": {"x"}, "EmptyStr": {}, "Nil": nil, "Missing": nil}
	for k, want := range cases {
		got := getStringSlice(obj, k)
		if (got == nil) != (want == nil) || len(got) != len(want) {
			t.Errorf("getStringSlice(%q) = %#v, want %#v", k, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("getStringSlice(%q) = %#v, want %#v", k, got, want)
			}
		}
	}
}

func TestObjectParamDecodesJSONOnly(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"Quarantine", "Quarantine"},
		{"Unlimited", "Unlimited"},
		{"true", "true"}, // scalars stay strings: only [..] / {..} are decoded
		{"7", "7"},
		{`["a","b"]`, []any{"a", "b"}},
		{` {"Key":"v"} `, map[string]any{"Key": "v"}},
		{`[not json`, `[not json`}, // malformed JSON is sent as-is
		{"", ""},
	}
	for _, c := range cases {
		if got := objectParam(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("objectParam(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestGetObjectJSONRoundTrips(t *testing.T) {
	obj := map[string]any{
		"Str":    "Quarantine",
		"Bool":   true,
		"Num":    float64(7),
		"List":   []any{"a", "b"},
		"Object": map[string]any{"Key": "v"},
		"Nil":    nil,
	}
	cases := map[string]string{
		"Str": "Quarantine", "Bool": "true", "Num": "7",
		"List": `["a","b"]`, "Object": `{"Key":"v"}`, "Nil": "", "Missing": "",
	}
	for k, want := range cases {
		if got := getObjectJSON(obj, k); got != want {
			t.Errorf("getObjectJSON(%q) = %q, want %q", k, got, want)
		}
	}
	// A jsonencode()-style config round-trips: write, read back, compare.
	for _, cfg := range []string{`["a","b"]`, `{"Key":"v"}`, "Quarantine"} {
		back := getObjectJSON(map[string]any{"X": objectParam(cfg)}, "X")
		if back != cfg {
			t.Errorf("round trip of %q read back as %q", cfg, back)
		}
	}
}

func TestListRemoveDelta(t *testing.T) {
	d := listRemoveDelta([]string{"a"})
	if d == nil || !reflect.DeepEqual(d.Remove, []string{"a"}) || d.Add != nil {
		t.Errorf("listRemoveDelta = %#v", d)
	}
}
