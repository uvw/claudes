package render

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"go.yaml.in/yaml/v4"
)

// Format is how a command prints what it lists or reports: text, JSON, or
// YAML. *Format is a pflag.Value, so a bad format fails when the flags are
// parsed.
type Format string

// The formats -o takes.
const (
	Text Format = "text"
	JSON Format = "json"
	YAML Format = "yaml"
)

// Formats lists what -o and output.format take.
var Formats = []string{string(Text), string(JSON), string(YAML)}

func (f Format) String() string { return string(f) }

// Type names the flag's value in help.
func (f *Format) Type() string { return "format" }

// Set takes a format by name.
func (f *Format) Set(v string) error {
	if !slices.Contains(Formats, v) {
		return fmt.Errorf("must be text, json, or yaml, not %q", v)
	}
	*f = Format(v)
	return nil
}

// Encode prints v as JSON, or as YAML for any other format. YAML is made from
// the JSON, so both use the same field names, in the same order.
func (f Format) Encode(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if f == JSON {
		_, err = fmt.Fprintf(w, "%s\n", data)
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	clearStyle(&node)
	// v3's layout, with no line folded, so a long directory or prompt stays on
	// one line for grep.
	d, err := yaml.NewDumper(w, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return err
	}
	if err := d.Dump(&node); err != nil {
		return err
	}
	return d.Close()
}

// clearStyle drops the flow and quoting styles kept from parsing JSON, so the
// output is plain block YAML. A string that would read as another type, such
// as an id of digits, is still quoted, because the encoder checks each tag.
func clearStyle(n *yaml.Node) {
	n.Style = 0
	for _, c := range n.Content {
		clearStyle(c)
	}
}
