// Package config reads and writes claudes' config file, config.yaml, and opens
// it in the user's editor.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/style"
)

// Config is everything config.yaml can set. A zero field means the built-in
// default.
type Config struct {
	Ping   Ping   `yaml:"ping"`
	Wait   Wait   `yaml:"wait"`
	Output Output `yaml:"output"`
	MCP    MCP    `yaml:"mcp"`
}

// Ping holds the prompt that --ping and claudes ping send.
type Ping struct {
	Text string `yaml:"text"`
}

// Wait holds how long --wait waits by default.
type Wait struct {
	Timeout Duration `yaml:"timeout"`
}

// Output holds the default -o format of ls and where, and how text tables
// shorten a long directory.
type Output struct {
	Format string `yaml:"format"`
	Dirs   string `yaml:"dirs"`
}

// DirsValues lists what output.dirs takes. The first is the default.
var DirsValues = []string{style.DirsMiddle, style.DirsFish, style.DirsFull}

// MCP holds the MCP server profiles that claudes mcp shows.
type MCP struct {
	// Profiles maps each profile's name to the servers that stay on in it.
	Profiles map[string]ServerNames `yaml:"profiles"`
}

// ServerNames is a list of MCP server names, each a YAML string.
type ServerNames []string

// UnmarshalYAML implements yaml.Unmarshaler. It refuses a number or a true,
// which YAML would otherwise turn into a name.
func (s *ServerNames) UnmarshalYAML(n *yaml.Node) error {
	if isNull(n) {
		*s = nil
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: a profile is a list of server names", n.Line)
	}
	names := make(ServerNames, 0, len(n.Content))
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return fmt.Errorf("line %d: a server name is a string; quote it", item.Line)
		}
		names = append(names, item.Value)
	}
	*s = names
	return nil
}

// Duration is a time.Duration written as "30m" or "1h30m".
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler. A null value is no setting.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if isNull(n) {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a duration such as 30m", n.Line, n.Value)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// Key is one setting, as `claudes config get` and `set` name it.
type Key struct {
	Name, About string
	get         func(Config) string
	// editOnly marks a setting that is not one string, which config set
	// cannot write.
	editOnly bool
}

// Keys lists every setting.
var Keys = []Key{
	{"ping.text", "the prompt that --ping and claudes ping send", func(c Config) string { return c.Ping.Text }, false},
	{"wait.timeout", "how long --wait waits, such as 30m", func(c Config) string {
		if c.Wait.Timeout == 0 {
			return ""
		}
		return time.Duration(c.Wait.Timeout).String()
	}, false},
	{"output.format", "the default -o of the commands that list or report: text, json, or yaml", func(c Config) string { return c.Output.Format }, false},
	{"output.dirs", "how text tables shorten a directory too long for its column: middle, fish, or full", func(c Config) string { return c.Output.Dirs }, false},
	{"mcp.profiles", "MCP server profiles: each name and the servers that stay on in it", profilesValue, true},
}

// profilesValue is mcp.profiles as one line of JSON, which YAML reads too, or
// "" when no profile is set.
func profilesValue(c Config) string {
	if len(c.MCP.Profiles) == 0 {
		return ""
	}
	profiles := map[string]ServerNames{}
	for name, servers := range c.MCP.Profiles {
		profiles[name] = append(ServerNames{}, servers...)
	}
	data, err := json.Marshal(profiles)
	if err != nil {
		return ""
	}
	return string(data)
}

// profileName is a simple word, so it reads the same in a flag and a table.
var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Path is $CLAUDES_CONFIG, or config.yaml in the directory paths.Config names.
func Path() (string, error) {
	if p := os.Getenv("CLAUDES_CONFIG"); p != "" {
		if !filepath.IsAbs(p) {
			return "", fmt.Errorf("CLAUDES_CONFIG must be an absolute path, not %q", p)
		}
		return p, nil
	}
	dir, err := paths.Config()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads path. A missing file is an empty config. An unknown key or a
// value claudes cannot use is an error, so a typo is not silently ignored.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	return parse(path, data)
}

func parse(path string, data []byte) (Config, error) {
	if err := oneDocument(path, data); err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}

	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	switch c.Output.Format {
	case "", "text", "json", "yaml":
	default:
		return fmt.Errorf("output.format must be text, json, or yaml, not %q", c.Output.Format)
	}
	if c.Output.Dirs != "" && !slices.Contains(DirsValues, c.Output.Dirs) {
		return fmt.Errorf("output.dirs must be middle, fish, or full, not %q", c.Output.Dirs)
	}
	if strings.HasPrefix(c.Ping.Text, "-") {
		return fmt.Errorf("ping.text starts with -, so claude would read it as an option")
	}
	if c.Wait.Timeout < 0 {
		return errors.New("wait.timeout cannot be negative")
	}
	for name, servers := range c.MCP.Profiles {
		if !profileName.MatchString(name) {
			return fmt.Errorf("mcp.profiles: %q is not a profile name: use letters, digits, - and _", name)
		}
		for _, s := range servers {
			if s == "" || strings.ContainsFunc(s, unicode.IsControl) {
				return fmt.Errorf("mcp.profiles.%s: %q is not a server name", name, s)
			}
		}
	}
	return nil
}

// Get returns a setting's value in c, or "" when it is not set.
func Get(c Config, name string) (string, error) {
	k, err := find(name)
	if err != nil {
		return "", err
	}
	return k.get(c), nil
}

// Set writes one setting into the file at path, keeping everything else in
// it, comments included. It checks the result before it writes.
func Set(path, name, value string) error {
	k, err := find(name)
	if err != nil {
		return err
	}
	if k.editOnly {
		return fmt.Errorf("%s is not one value; change it with claudes config edit", name)
	}
	// Write through a link, such as one into a dotfiles repository, rather
	// than replacing the link with a file.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := oneDocument(path, data); err != nil {
		return err
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	var out bytes.Buffer
	switch {
	case doc.Kind == 0 || isNull(doc.Content[0]):
		// No settings yet. Keep the comments, such as the template's, and
		// add the setting after them.
		for line := range strings.Lines(string(data)) {
			if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
				out.WriteString(strings.TrimRight(line, "\r\n") + "\n")
			}
		}
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	case doc.Content[0].Kind != yaml.MappingNode:
		return fmt.Errorf("%s holds no list of settings, so claudes cannot add one", path)
	}
	setPath(doc.Content[0], strings.Split(name, "."), value)
	d, err := yaml.NewDumper(&out, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return err
	}
	if err := d.Dump(&doc); err != nil {
		return err
	}
	if err := d.Close(); err != nil {
		return err
	}
	if _, err := parse(path, out.Bytes()); err != nil {
		return err
	}
	return files.WriteFileAll(path, out.Bytes(), 0o600)
}

// Create writes Template to path when no file is there yet.
func Create(path string) error {
	if Exists(path) {
		return nil
	}
	return files.WriteFileAll(path, Template(), 0o600)
}

// Exists reports whether there is a file at path, so the built-in defaults are
// not all that applies.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// Template is what `claudes config edit` starts a new file with.
func Template() []byte {
	var b strings.Builder
	b.WriteString("# claudes settings. Flags always win over these.\n")
	b.WriteString("# Every key is optional; remove the # to set one.\n\n")
	last := ""
	for _, k := range Keys {
		section, field, _ := strings.Cut(k.Name, ".")
		if section != last {
			fmt.Fprintf(&b, "# %s:\n", section)
			last = section
		}
		fmt.Fprintf(&b, "#   %s: ...   # %s\n", field, k.About)
	}
	return []byte(b.String())
}

func find(name string) (Key, error) {
	if i := slices.IndexFunc(Keys, func(k Key) bool { return k.Name == name }); i >= 0 {
		return Keys[i], nil
	}
	names := make([]string, 0, len(Keys))
	for _, k := range Keys {
		names = append(names, k.Name)
	}
	return Key{}, fmt.Errorf("no setting %q. Settings: %s", name, strings.Join(names, ", "))
}

// isNull reports whether n is a document that is empty in YAML's sense: ~,
// null, or nothing after ---.
func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// setPath sets keys[0].keys[1]... to value inside a mapping node, adding the
// mappings it needs. Every setting is a string, so a value such as null or 30
// is quoted when it would read as something else.
func setPath(m *yaml.Node, keys []string, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != keys[0] {
			continue
		}
		if len(keys) == 1 {
			m.Content[i+1] = str(value)
			return
		}
		if m.Content[i+1].Kind != yaml.MappingNode {
			m.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
		}
		setPath(m.Content[i+1], keys[1:], value)
		return
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: keys[0]}
	if len(keys) == 1 {
		m.Content = append(m.Content, key, str(value))
		return
	}
	child := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, key, child)
	setPath(child, keys[1:], value)
}

// oneDocument refuses a file of several YAML documents: claudes would read
// only the first, and config set would drop the rest.
func oneDocument(path string, data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for i := 0; ; i++ {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if i > 0 {
			return fmt.Errorf("%s holds more than one YAML document; keep the settings in one", path)
		}
	}
}

func str(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
