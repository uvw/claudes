package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
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

// dirCell is a table cell that shows the directory path. Every path a table
// shows goes through it, so writeTable can shorten and fade it.
func dirCell(path string) string { return style.DirCell(homeShort(path)) }

// tableFit says how writeTable fits a table to the line.
type tableFit struct {
	// margin is how many columns go before each line.
	margin int
	// fitLast cuts the last column to what is left of the terminal's width
	// after the directories, or to lastCap off a terminal when lastCap is not
	// 0.
	fitLast bool
	lastCap int
}

// writeTable writes rows, the first being the header, as style.Table does,
// after style.Fit fits them to Out's terminal and output.dirs.
func (app *App) writeTable(w io.Writer, rows [][]string, fit tableFit) error {
	style.Fit(rows, style.Layout{
		Width: app.width(), Margin: fit.margin, Dirs: app.dirs(), FitLast: fit.fitLast, LastCap: fit.lastCap,
	}, app.colors.Faint)
	return style.Table(w, rows)
}

// dirIn is path as a line of text shows it, such as a heading, shortened as a
// table shortens a directory. used is how many columns the rest of the line
// takes.
func (app *App) dirIn(path string, used int) string {
	return style.ShortDir(homeShort(path), style.DirRoom(app.width(), used), app.dirs())
}

// dirs is output.dirs, or its default.
func (app *App) dirs() string { return cmp.Or(app.Config.Output.Dirs, style.DirsMiddle) }

// width is how many columns the terminal Out writes to has, or 0 when Out is
// not a terminal.
func (app *App) width() int {
	if app.Width == nil {
		return 0
	}
	return max(app.Width(), 0)
}

// homeShort is path, printable, with ~ for the home directory.
func homeShort(path string) string {
	path = shell.Printable(path)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, ok := relWithin(home, path)
	switch {
	case !ok:
		return path
	case rel == ".":
		return "~"
	}
	return "~" + string(filepath.Separator) + rel
}

// printDirs prints one directory bare, so cd "$(claudes where NAME)" works, or
// several as name, id, and directory, so sessions that share a name stay apart.
func (app *App) printDirs(found [][3]string) error {
	if len(found) == 1 {
		_, err := fmt.Fprintln(app.Out, shell.Printable(found[0][2]))
		return err
	}
	table := make([][]string, 0, len(found))
	for _, f := range found {
		table = append(table, []string{shell.Printable(f[0]), f[1], dirCell(f[2])})
	}
	return app.writeTable(app.Out, table, tableFit{})
}
