package cswap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// listTimeout bounds cswap list --json, which asks Anthropic for each
// account's usage.
const listTimeout = time.Minute

// List runs `cswap list --json` with the cswap at binary and returns what it
// printed. It starts Python and asks for each account's usage, which cswap
// rate-limits, so only claudes doctor runs it.
func List(ctx context.Context, binary string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "list", "--json")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("cswap list --json timed out after %s", listTimeout)
		}
		return stdout.Bytes(), fmt.Errorf("cswap list --json failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Listed is one account as cswap list --json prints it.
type Listed struct {
	Number int    `json:"number"`
	Email  string `json:"email"`
	Alias  string `json:"alias"`
}

// ParseList reads what cswap list --json printed.
func ParseList(data []byte) (active string, accounts []Listed, err error) {
	var out struct {
		Active   *int     `json:"activeAccountNumber"`
		Accounts []Listed `json:"accounts"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", nil, fmt.Errorf("cswap list --json printed something other than its list of accounts: %w", err)
	}
	if out.Error != nil {
		return "", nil, fmt.Errorf("cswap list --json reported an error: %s", out.Error.Message)
	}
	if out.Active != nil {
		active = strconv.Itoa(*out.Active)
	}
	return active, out.Accounts, nil
}

// Differences lists where what cswap list --json printed differs from seq,
// which claudes read from sequence.json: an account only one of them has, an
// email or alias that differs, or another active account.
func Differences(seq Sequence, active string, listed []Listed) []string {
	var out []string
	if active != seq.Active {
		out = append(out, fmt.Sprintf("cswap's active account is %s, but sequence.json says %s", orNone(active), orNone(seq.Active)))
	}
	seen := map[string]bool{}
	for _, l := range listed {
		n := strconv.Itoa(l.Number)
		seen[n] = true
		a, ok := seq.Account(n)
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("cswap lists account %s (%s), which sequence.json lacks", n, l.Email))
		case a.Email != l.Email:
			out = append(out, fmt.Sprintf("cswap lists account %s as %s, but sequence.json as %s", n, l.Email, a.Email))
		case a.Alias != l.Alias:
			out = append(out, fmt.Sprintf("cswap lists account %s with alias %s, but sequence.json with %s", n, orNone(l.Alias), orNone(a.Alias)))
		}
	}
	for _, a := range seq.Accounts {
		if !seen[a.Number] {
			out = append(out, fmt.Sprintf("sequence.json holds account %s (%s), which cswap does not list", a.Number, a.Email))
		}
	}
	slices.Sort(out)
	return out
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
