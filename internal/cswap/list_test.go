package cswap

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// helperEnv makes the test binary stand in for cswap: "list" prints a list,
// and "fail" prints cswap's error envelope and exits 1.
const helperEnv = "CLAUDES_TEST_CSWAP"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "list":
		fmt.Print(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	case "fail":
		fmt.Print(`{"schemaVersion":1,"error":{"type":"ConfigError","message":"no accounts"}}`)
		fmt.Fprint(os.Stderr, "notice")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestListRunsCswapListJSON(t *testing.T) {
	t.Setenv(helperEnv, "list")
	out, err := List(t.Context(), os.Args[0])
	if err != nil || string(out) != "list --json" {
		t.Fatalf("out %q, err %v", out, err)
	}
}

func TestListKeepsWhatAFailedRunPrinted(t *testing.T) {
	t.Setenv(helperEnv, "fail")
	out, err := List(t.Context(), os.Args[0])
	if err == nil || !strings.Contains(err.Error(), "notice") {
		t.Fatalf("err %v", err)
	}
	if _, _, perr := ParseList(out); perr == nil || !strings.Contains(perr.Error(), "no accounts") {
		t.Fatalf("parsed %v", perr)
	}
}
