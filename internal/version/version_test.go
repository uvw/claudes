package version

import "testing"

func TestLess(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"2.1.322", "2.1.325", true},
		{"2.1.325", "2.1.325", false},
		{"2.1.330", "2.1.325", false},
		{"2.1", "2.1.1", true},
		{"2.10.0", "2.9.9", false},
		{"v2.1.3", "2.1.4", true},
	} {
		if got := Less(tc.a, tc.b); got != tc.want {
			t.Errorf("Less(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareAndMinor(t *testing.T) {
	if Compare("2.1.3", "2.1.3") != 0 || Compare("2.1", "2.1.0") != 0 || Compare("2.2", "2.1.999") != 1 || Compare("v1.9", "2.0") != -1 {
		t.Fatal("Compare")
	}
	for in, want := range map[string]string{"2.1.325": "2.1", "v3.0": "3.0", "7": "7", " 2.1.1 ": "2.1"} {
		if got := Minor(in); got != want {
			t.Errorf("Minor(%q) = %q, want %q", in, got, want)
		}
	}
}
