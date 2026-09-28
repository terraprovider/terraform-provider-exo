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
