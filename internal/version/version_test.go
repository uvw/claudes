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

func TestBuildPrefersTheReleaseVersion(t *testing.T) {
	defer func(v, c string) { Version, Commit = v, c }(Version, Commit)
	Version, Commit = "0.3.0", "abc1234"
	if got := Build(); got != "0.3.0 (abc1234)" {
		t.Fatalf("got %q", got)
	}
	Commit = ""
	if got := Build(); got != "0.3.0" {
		t.Fatalf("got %q", got)
	}
	Version = "dev"
	if got := Build(); got == "" {
		t.Fatal("a source build has no version")
	}
}
