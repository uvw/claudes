// Package cswap reads the accounts and profiles of claude-swap, cswap for
// short, which runs Claude Code under several accounts side by side. cswap run
// starts claude with CLAUDE_CONFIG_DIR set to a profile of the account, a
// directory under the backup directory's sessions. claudes reads cswap's files
// and never writes them.
package cswap

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Account is one account in sequence.json.
type Account struct {
	// Number is the account's slot, as sequence.json keys it, such as "2".
	Number           string `json:"-"`
	Email            string `json:"email"`
	UUID             string `json:"uuid"`
	OrganizationUUID string `json:"organizationUuid"`
	OrganizationName string `json:"organizationName"`
	Added            string `json:"added"`
	Alias            string `json:"alias,omitempty"`
}

// Sequence is sequence.json: the accounts cswap manages.
type Sequence struct {
	// Active is the number of the account ~/.claude is logged in as, or ""
	// when none is.
	Active string
	// Accounts are in number order.
	Accounts []Account
}

// Account is the account numbered n, if there is one.
func (s Sequence) Account(n string) (Account, bool) {
	i := slices.IndexFunc(s.Accounts, func(a Account) bool { return a.Number == n })
	if i < 0 {
		return Account{}, false
	}
	return s.Accounts[i], true
}

// ReadSequence reads sequence.json in the backup directory dir. A missing
// file is an error that wraps fs.ErrNotExist: cswap manages no account yet.
func ReadSequence(dir string) (Sequence, error) {
	path := filepath.Join(dir, "sequence.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Sequence{}, err
	}
	var raw struct {
		Active   *json.Number       `json:"activeAccountNumber"`
		Accounts map[string]Account `json:"accounts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Sequence{}, fmt.Errorf("%s is not cswap's list of accounts: %w", path, err)
	}
	var s Sequence
	if raw.Active != nil {
		s.Active = raw.Active.String()
	}
	for n, a := range raw.Accounts {
		if !isNumber(n) {
			return Sequence{}, fmt.Errorf("%s numbers an account %q", path, n)
		}
		a.Number = n
		s.Accounts = append(s.Accounts, a)
	}
	slices.SortFunc(s.Accounts, func(a, b Account) int { return compareNumbers(a.Number, b.Number) })
	return s, nil
}

func compareNumbers(a, b string) int {
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return x - y
}

// isNumber reports whether s is an account number: digits only.
func isNumber(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
}

// Slug is the email as a profile's name spells it: every character but an
// ASCII letter, digit, '.', '_', or '-' becomes '_'. cswap first puts the
// email in Unicode NFC form, which claudes does not: Claude Code's emails come
// in that form already, and a mismatch only leaves the profile skipped, which
// claudes doctor reports.
func Slug(email string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return r
		}
		return '_'
	}, email)
}

// ProfileName is the name of the profile directory cswap run uses for a.
func ProfileName(a Account) string { return a.Number + "-" + Slug(a.Email) }

// Profile is one directory under the backup directory's sessions.
type Profile struct {
	// Dir is the profile, spelled as cswap run sets CLAUDE_CONFIG_DIR to it.
	Dir string
	// Account is the account it belongs to, when Skipped is "".
	Account Account
	// Skipped says why claudes does not read the profile as an account, or
	// is "" when it does.
	Skipped string
	// Jobs reports whether the profile holds a jobs directory, so it can have
	// background sessions.
	Jobs bool
}

// Profiles lists the profiles in the backup directory dir, in name order. One
// counts when its leading number names an account in seq, the rest of its name
// is that account's Slug, and it holds .claude.json or jobs. Any other is
// listed with why it is skipped: an account cswap removed, or a slot it gave
// another email, leaves a stale profile behind. A missing sessions directory
// holds none.
func Profiles(dir string, seq Sequence) ([]Profile, error) {
	root := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := Profile{Dir: filepath.Join(root, e.Name())}
		p.Jobs = isDir(filepath.Join(p.Dir, "jobs"))
		n, _, _ := strings.Cut(e.Name(), "-")
		a, known := seq.Account(n)
		switch {
		case !isNumber(n):
			p.Skipped = "its name does not start with an account number"
		case !known:
			p.Skipped = "sequence.json has no account " + n
		case e.Name() != ProfileName(a):
			p.Skipped = fmt.Sprintf("account %s is %s now", n, a.Email)
		case !p.Jobs && !exists(filepath.Join(p.Dir, ".claude.json")):
			p.Skipped = "it holds neither .claude.json nor jobs"
		default:
			p.Account = a
		}
		out = append(out, p)
	}
	return out, nil
}

// Lookup finds the accounts id names, as cswap resolves an account: a number,
// then an alias in any case, then an email. An email that names several
// accounts is an error that asks for the number. No match is no accounts and
// no error.
func (s Sequence) Lookup(id string) ([]Account, error) {
	if isNumber(id) {
		if a, ok := s.Account(id); ok {
			return []Account{a}, nil
		}
		return nil, nil
	}
	if i := slices.IndexFunc(s.Accounts, func(a Account) bool { return a.Alias != "" && strings.EqualFold(a.Alias, id) }); i >= 0 {
		return []Account{s.Accounts[i]}, nil
	}
	var found []Account
	for _, a := range s.Accounts {
		if a.Email == id {
			found = append(found, a)
		}
	}
	if len(found) > 1 {
		names := make([]string, len(found))
		for i, a := range found {
			names[i] = a.Number + " (" + cmp.Or(a.OrganizationName, "personal") + ")"
		}
		return nil, fmt.Errorf("%s is the email of accounts %s: name one by its number", id, strings.Join(names, ", "))
	}
	return found, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
