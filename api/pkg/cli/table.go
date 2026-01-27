package cli

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// NewSimpleTable creates a simple borderless table for CLI output.
// This provides a consistent look across all CLI list commands.
func NewSimpleTable(w io.Writer, header []string) *tablewriter.Table {
	// Create a table with minimal/no borders for clean CLI output
	table := tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Symbols: tw.NewSymbols(tw.StyleNone),
			Borders: tw.Border{
				Top:    tw.Off,
				Bottom: tw.Off,
				Left:   tw.Off,
				Right:  tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows:    tw.Off,
					BetweenColumns: tw.Off,
				},
			},
		})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoFormat(tw.On),
		tablewriter.WithTrimSpace(tw.Off),
	)

	// Set header - convert []string to variadic any
	headerAny := make([]any, len(header))
	for i, h := range header {
		headerAny[i] = h
	}
	table.Header(headerAny...)

	return table
}

// AppendRow appends a row to the table
func AppendRow(table *tablewriter.Table, row []string) error {
	// Convert []string to variadic interface{}
	rowAny := make([]interface{}, len(row))
	for i, v := range row {
		rowAny[i] = v
	}
	return table.Append(rowAny...)
}

// RenderTable renders the table and returns any error
func RenderTable(table *tablewriter.Table) error {
	return table.Render()
}
