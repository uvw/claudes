package render

import (
	"strings"
	"testing"
)

func TestYAMLQuotesOnlyWhatNeedsIt(t *testing.T) {
	var buf strings.Builder
	type item struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
	}
	if err := YAML.Encode(&buf, []item{{Name: "alpha", ID: "12345678", Cwd: `C:\x`}}); err != nil {
		t.Fatal(err)
	}
	want := "- name: alpha\n  id: \"12345678\"\n  cwd: C:\\x\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestJSONIsIndentedAndEndsInANewline(t *testing.T) {
	var buf strings.Builder
	if err := JSON.Encode(&buf, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"a\": 1\n}\n"; buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
	if err := JSON.Encode(&buf, func() {}); err == nil {
		t.Fatal("encoded a func")
	}
}

func TestFormatTakesOnlyItsNames(t *testing.T) {
	var f Format
	for _, name := range Formats {
		if err := f.Set(name); err != nil || f.String() != name {
			t.Errorf("Set(%q): %v, now %q", name, err, f)
		}
	}
	if err := f.Set("xml"); err == nil || !strings.Contains(err.Error(), "must be text, json, or yaml") || f != YAML {
		t.Errorf("Set(xml): %v, now %q", err, f)
	}
	if f.Type() != "format" {
		t.Errorf("Type %q", f.Type())
	}
}
