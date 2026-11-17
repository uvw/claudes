// Package config reads and writes claudes' config file, config.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/safefile"
)

// Config is everything config.yaml can set. A zero field means the built-in
// default.
type Config struct {
	Ping   Ping   `yaml:"ping"`
	Wait   Wait   `yaml:"wait"`
	Output Output `yaml:"output"`
}

// Ping holds the prompt that --ping and claudes ping send.
type Ping struct {
	Text string `yaml:"text"`
}

// Wait holds how long --wait waits by default.
type Wait struct {
	Timeout Duration `yaml:"timeout"`
}

// Output holds the default -o format of ls and where.
type Output struct {
	Format string `yaml:"format"`
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
}

// Keys lists every setting.
var Keys = []Key{
	{"ping.text", "the prompt that --ping and claudes ping send", func(c Config) string { return c.Ping.Text }},
	{"wait.timeout", "how long --wait waits, such as 30m", func(c Config) string {
		if c.Wait.Timeout == 0 {
			return ""
		}
		return time.Duration(c.Wait.Timeout).String()
	}},
	{"output.format", "the default -o of commands that print sessions: text, json, or yaml", func(c Config) string { return c.Output.Format }},
}

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
	if strings.HasPrefix(c.Ping.Text, "-") {
		return fmt.Errorf("ping.text starts with -, so claude would read it as an option")
	}
	if c.Wait.Timeout < 0 {
		return errors.New("wait.timeout cannot be negative")
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
	if _, err := find(name); err != nil {
		return err
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
	return safefile.WriteFileAll(path, out.Bytes(), 0o600)
}

// Create writes Template to path when no file is there yet.
func Create(path string) error {
	if Exists(path) {
		return nil
	}
	return safefile.WriteFileAll(path, Template(), 0o600)
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
	for _, k := range Keys {
		section, field, _ := strings.Cut(k.Name, ".")
		fmt.Fprintf(&b, "# %s:\n#   %s: ...   # %s\n", section, field, k.About)
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
