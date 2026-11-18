package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"
)

func TestTableLinesUpAsTabwriterDoes(t *testing.T) {
	rows := [][]string{
		{"RESULT", "CHECK", "DETAIL"},
		{"ok", "claude", "2.1.330 at /bin/claude"},
		{"fail", "agents --json", "first problem"},
		{"", "", "second problem"},
		{"warn", "", ""},
	}
	var want bytes.Buffer
	tw := tabwriter.NewWriter(&want, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()

	var plain bytes.Buffer
	if err := Table(&plain, rows); err != nil {
		t.Fatal(err)
	}
	if plain.String() != want.String() {
		t.Fatalf("got:\n%s\nwant:\n%s", plain.String(), want.String())
	}

	colored := make([][]string, len(rows))
	for i, r := range rows {
		colored[i] = []string{On.Red(r[0]), r[1], On.Faint(r[2])}
	}
	var out bytes.Buffer
	if err := Table(&out, colored); err != nil {
		t.Fatal(err)
	}
	if !sgr.MatchString(out.String()) {
		t.Fatalf("no colors:\n%q", out.String())
	}
	if got := sgr.ReplaceAllString(out.String(), ""); got != want.String() {
		t.Fatalf("colored table without its colors:\n%s\nwant:\n%s", got, want.String())
	}
}
