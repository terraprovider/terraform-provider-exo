package provider

import "testing"

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
