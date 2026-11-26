// Package accounts lists the Claude Code directories claudes manages, one for
// each account: Claude Code's own, the profiles cswap runs sessions in, and
// the directories config.yaml adds. It names each one and finds the ones
// --account names.
package accounts

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/cswap"
	"github.com/uvw/claudes/internal/paths"
)

// Where an account came from, as Account.From lists it.
const (
	FromEnv    = "CLAUDE_CONFIG_DIR"
	FromHome   = "~/.claude"
	FromCswap  = "cswap"
	FromConfig = "config.yaml"
)

// Account is one Claude Code directory.
type Account struct {
	// Name is how claudes shows the account: its config.yaml name, else its
	// cswap alias, else its cswap email, else its directory.
	Name string
	Dir  string
	// ConfigFile is its .claude.json.
	ConfigFile string
	// ConfigDir is how claude reaches it. The account claudes runs as has
	// the zero value.
	ConfigDir claude.ConfigDir
	// From lists where claudes found it, in the order it did.
	From []string
	// Cswap is the cswap account it is logged in as, if any.
	Cswap *cswap.Account
	// configName is its name in config.yaml, if it has one.
	configName string
}

// Options are what Find looks at.
type Options struct {
	// Home is the user's home directory, which holds ~/.claude.
	Home string
	// Env is $CLAUDE_CONFIG_DIR, an absolute path, or "".
	Env string
	// CswapDir is cswap's backup directory, or "" when claudes leaves cswap
	// out: it is not on PATH, or config.yaml turns it off.
	CswapDir string
	// Dirs are config.yaml's accounts.dirs, by name, each an absolute path.
	Dirs map[string]string
}

// Found is what Find found.
type Found struct {
	// Home is ~/.claude.
	Home string
	// Accounts come with the one claudes runs as first, then ~/.claude, then
	// cswap's profiles by number, then the rest of config.yaml's by name.
	Accounts []Account
	// Sequence is cswap's sequence.json, when claudes read one.
	Sequence cswap.Sequence
	// Cswap reports whether claudes read sequence.json.
	Cswap bool
	// Skipped are the cswap profiles claudes does not read as accounts.
	Skipped []cswap.Profile
	// Problems are what kept claudes from reading part of it, such as a
	// sequence.json it cannot parse. The accounts it could read still count.
	Problems []error
}

// Find lists the accounts. The directory claudes runs with, $CLAUDE_CONFIG_DIR
// or ~/.claude, is always one. With cswap in use, ~/.claude is one, and each
// profile that names an account in sequence.json and holds .claude.json or
// jobs. config.yaml's accounts.dirs name any of them, by directory, or add
// directories of their own.
func Find(o Options) Found {
	home := filepath.Join(o.Home, ".claude")
	f := Found{Home: home}
	own := Account{Dir: home, ConfigFile: filepath.Join(o.Home, ".claude.json"), From: []string{FromHome}}
	if o.Env != "" {
		own = Account{Dir: o.Env, ConfigFile: filepath.Join(o.Env, ".claude.json"), From: []string{FromEnv}}
	}
	f.Accounts = []Account{own}
	if o.CswapDir != "" {
		f.readCswap(o.CswapDir, home, o.Home)
	}
	for _, name := range slices.Sorted(maps.Keys(o.Dirs)) {
		dir := o.Dirs[name]
		if !paths.IsDir(dir) {
			f.Problems = append(f.Problems, fmt.Errorf("accounts.dirs.%s: %s is not a directory, so claudes leaves it out", name, dir))
			continue
		}
		a := Account{Dir: dir, ConfigFile: filepath.Join(dir, ".claude.json"), ConfigDir: claude.ConfigDir{Path: dir}, From: []string{FromConfig}, configName: name}
		if paths.Same(dir, home) {
			a.ConfigFile, a.ConfigDir = filepath.Join(o.Home, ".claude.json"), claude.ConfigDir{Unset: true}
		}
		f.add(a)
	}
	for i := range f.Accounts {
		f.Accounts[i].Name = f.Accounts[i].name(o.Home)
	}
	return f
}

// readCswap adds ~/.claude, logged in as cswap's active account, and each
// profile that counts.
func (f *Found) readCswap(dir, home, userHome string) {
	seq, err := cswap.ReadSequence(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		f.Problems = append(f.Problems, fmt.Errorf("cannot read cswap's accounts, so claudes leaves its profiles out: %w", err))
		return
	}
	f.Sequence, f.Cswap = seq, true
	a := Account{Dir: home, ConfigFile: filepath.Join(userHome, ".claude.json"), ConfigDir: claude.ConfigDir{Unset: true}, From: []string{FromHome}}
	if active, ok := seq.Account(seq.Active); ok {
		a.Cswap = &active
	}
	f.add(a)
	profiles, err := cswap.Profiles(dir, seq)
	if err != nil {
		f.Problems = append(f.Problems, fmt.Errorf("cannot list cswap's profiles, so claudes leaves them out: %w", err))
		return
	}
	for _, p := range profiles {
		if p.Skipped != "" {
			f.Skipped = append(f.Skipped, p)
			continue
		}
		f.add(Account{Dir: p.Dir, ConfigFile: filepath.Join(p.Dir, ".claude.json"), ConfigDir: claude.ConfigDir{Path: p.Dir}, From: []string{FromCswap}, Cswap: &p.Account})
	}
}

// add adds a, or merges it into the account that already has its directory:
// that one keeps its ConfigDir, and takes a's config.yaml name and cswap
// account.
func (f *Found) add(a Account) {
	i := slices.IndexFunc(f.Accounts, func(x Account) bool { return paths.Same(x.Dir, a.Dir) })
	if i < 0 {
		f.Accounts = append(f.Accounts, a)
		return
	}
	x := &f.Accounts[i]
	for _, from := range a.From {
		if !slices.Contains(x.From, from) {
			x.From = append(x.From, from)
		}
	}
	if a.configName != "" {
		x.configName = a.configName
	}
	if a.Cswap != nil {
		x.Cswap = a.Cswap
	}
}

func (a Account) name(home string) string {
	switch {
	case a.configName != "":
		return a.configName
	case a.Cswap != nil && a.Cswap.Alias != "":
		return a.Cswap.Alias
	case a.Cswap != nil:
		return a.Cswap.Email
	}
	if rel, ok := paths.RelWithin(home, a.Dir); ok && rel != "." {
		return "~/" + filepath.ToSlash(rel)
	}
	return a.Dir
}

// Resolve finds the accounts token names: a config.yaml name; or, as cswap
// takes them, an account number, an alias in any case, or an email; or an
// account's name or directory. A cswap account that ~/.claude and a profile
// both hold names both.
func (f Found) Resolve(token string) ([]Account, error) {
	if i := slices.IndexFunc(f.Accounts, func(a Account) bool { return a.configName == token }); i >= 0 {
		return []Account{f.Accounts[i]}, nil
	}
	if f.Cswap {
		found, err := f.Sequence.Lookup(token)
		if err != nil {
			return nil, err
		}
		if len(found) > 0 {
			n := found[0].Number
			out := slices.DeleteFunc(slices.Clone(f.Accounts), func(a Account) bool { return a.Cswap == nil || a.Cswap.Number != n })
			if len(out) == 0 {
				return nil, fmt.Errorf("cswap account %s (%s) has no profile claudes reads, and ~/.claude is not logged in as it: see claudes doctor", n, found[0].Email)
			}
			return out, nil
		}
	}
	out := slices.DeleteFunc(slices.Clone(f.Accounts), func(a Account) bool { return a.Name != token && !(filepath.IsAbs(token) && paths.Same(a.Dir, token)) })
	if len(out) == 0 {
		return nil, fmt.Errorf("no account %q. Accounts: %s", token, strings.Join(f.Names(), ", "))
	}
	return out, nil
}

// ConfigDirOf is how claude reaches the Claude Code directory dir: as the
// account with that directory, or else, for a directory no account has now,
// with CLAUDE_CONFIG_DIR unset for ~/.claude and set to dir for any other.
func (f Found) ConfigDirOf(dir string) claude.ConfigDir {
	if i := slices.IndexFunc(f.Accounts, func(a Account) bool { return a.Dir == dir }); i >= 0 {
		return f.Accounts[i].ConfigDir
	}
	if dir == f.Home {
		return claude.ConfigDir{Unset: true}
	}
	return claude.ConfigDir{Path: dir}
}

// Names are the accounts' names, each once, in order.
func (f Found) Names() []string {
	var out []string
	for _, a := range f.Accounts {
		if !slices.Contains(out, a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}
