package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

// outputFlag is -o/--output for the commands that print sessions. It is a
// pflag.Value, so a bad format fails when the flags are parsed.
type outputFlag struct{ format string }

var outputFormats = []string{"text", "json", "yaml"}

// add registers -o, defaulting to format, or to text when format is "".
// config.Load has already checked a format from config.yaml.
func (o *outputFlag) add(cmd *cobra.Command, format string) {
	o.format = cmp.Or(format, "text")
	cmd.Flags().VarP(o, "output", "o", "print as text, json, or yaml")
	cmd.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(outputFormats, cobra.ShellCompDirectiveNoFileComp))
}

func (o *outputFlag) String() string { return o.format }

func (o *outputFlag) Type() string { return "format" }

func (o *outputFlag) Set(v string) error {
	if !slices.Contains(outputFormats, v) {
		return fmt.Errorf("must be text, json, or yaml, not %q", v)
	}
	o.format = v
	return nil
}

// encode prints v as JSON or YAML. YAML is made from the JSON, so both use the
// same field names, in the same order.
func (o *outputFlag) encode(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if o.format == "json" {
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
