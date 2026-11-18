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

// outputFlag is -o/--output for the commands that list or report. It is a
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

// dirCell is a table cell that shows the directory path, in a column headed
// dirHeader. Every path a table shows goes through it.
func dirCell(path string) string { return homeShort(path) }

// dirHeader heads a table's directory column, which writeTable shortens to fit
// and fades.
const dirHeader = "DIR"

// nameHeader and branchHeader head the columns of nameCell and branchCell,
// which writeTable cuts further when a line is wider than the terminal.
const nameHeader, branchHeader = "NAME", "BRANCH"

// tableFit says how writeTable fits a table to the line.
type tableFit struct {
	// margin is how many columns go before each line.
	margin int
	// fitLast cuts the last column to what is left of the terminal's width,
	// or to lastCap off a terminal when lastCap is not 0.
	fitLast bool
	lastCap int
}

// writeTable writes rows, the first being the header, as style.Table does,
// after style.Fit fits them to Out's terminal and output.dirs. While a line is
// wider than the terminal, it cuts the last column with fitLast, then each
// directory, then each name, then each branch.
func (app *App) writeTable(w io.Writer, rows [][]string, fit tableFit) error {
	var cols []style.Column
	if len(rows) > 0 {
		last := len(rows[0]) - 1
		if fit.fitLast {
			cols = append(cols, style.Column{Index: last, Cut: style.CutEnd, Min: style.MinLast, Max: fit.lastCap})
		}
		for _, c := range []struct {
			header string
			col    style.Column
		}{
			{dirHeader, style.Column{Cut: style.CutDir, Min: style.MinDir, Max: style.MaxDir}},
			{nameHeader, style.Column{Cut: style.CutEnd, Min: minName}},
			{branchHeader, style.Column{Cut: style.CutBranch, Min: minBranch}},
		} {
			for i, h := range rows[0] {
				if h == c.header && (i != last || !fit.fitLast) {
					c.col.Index = i
					cols = append(cols, c.col)
				}
			}
		}
	}
	style.Fit(rows, style.Layout{Width: app.width(), Margin: fit.margin, Dirs: app.dirs(), Columns: cols}, app.colors.Faint)
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

// dirRow is one row of the table where and session where print.
type dirRow struct{ id, dir, name string }

// printDirs prints one directory bare, so cd "$(claudes where NAME)" works, or
// several as a table of id, directory, and name, so sessions that share a name
// stay apart.
func (app *App) printDirs(found []dirRow) error {
	if len(found) == 1 {
		_, err := fmt.Fprintln(app.Out, shell.Printable(found[0].dir))
		return err
	}
	table := [][]string{{"ID", "DIR", "NAME"}}
	for _, f := range found {
		table = append(table, []string{f.id, dirCell(f.dir), nameCell(f.name)})
	}
	return app.writeTable(app.Out, table, tableFit{})
}
