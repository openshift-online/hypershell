package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type TableBuilder struct {
	printer       *Printer
	name          string
	specs         []string
	learning      bool
	learningLimit int
}

type Table struct {
	printer       *Printer
	name          string
	columns       []*Column
	learning      bool
	learningLimit int
	learningRows  [][]string
}

type Column struct {
	table  *Table
	name   string
	header string
	learn  bool
	width  int
}

func (p *Printer) NewTable() *TableBuilder {
	return &TableBuilder{
		printer:       p,
		learning:      true,
		learningLimit: 100,
	}
}

func (b *TableBuilder) Name(value string) *TableBuilder {
	b.name = value
	return b
}

func (b *TableBuilder) Columns(specs ...string) *TableBuilder {
	b.specs = append(b.specs, specs...)
	return b
}

func (b *TableBuilder) Build(ctx context.Context) (result *Table, err error) {
	if b.printer == nil {
		err = fmt.Errorf("printer is mandatory")
		return
	}
	if b.name == "" {
		err = fmt.Errorf("name is mandatory")
		return
	}
	if len(b.specs) == 0 {
		err = fmt.Errorf("at least one column is required")
		return
	}

	columnNames := make([]string, 0, len(b.specs))
	for _, specs := range b.specs {
		specChunks := strings.Split(specs, ",")
		for _, specChunk := range specChunks {
			columnName := strings.TrimSpace(specChunk)
			if columnName != "" {
				columnNames = append(columnNames, columnName)
			}
		}
	}

	table := &Table{
		printer:       b.printer,
		name:          b.name,
		columns:       make([]*Column, len(columnNames)),
		learning:      b.learning,
		learningLimit: b.learningLimit,
	}

	for i, name := range columnNames {
		header := strings.ReplaceAll(name, ".", " ")
		header = strings.ReplaceAll(header, "_", " ")
		header = strings.ToUpper(header)
		table.columns[i] = &Column{
			table:  table,
			name:   name,
			header: header,
			learn:  true,
			width:  len(name),
		}
	}

	result = table
	return
}

func (t *Table) WriteRow(rowValues []interface{}) error {
	valueCount := len(rowValues)
	columnCount := len(t.columns)
	if valueCount != columnCount {
		return fmt.Errorf(
			"table '%s' has %d columns, but %d values have been given",
			t.name, columnCount, valueCount,
		)
	}

	rowData := make([]string, columnCount)
	for i, columnValue := range rowValues {
		if columnValue != nil {
			rowData[i] = fmt.Sprintf("%v", columnValue)
		} else {
			rowData[i] = ""
		}
	}

	accumulated, err := t.accumulateRow(rowData)
	if err != nil {
		return err
	}
	if accumulated {
		return nil
	}

	return t.writeRow(rowData)
}

func (t *Table) accumulateRow(rowData []string) (accumulated bool, err error) {
	if !t.learning {
		return false, nil
	}

	if len(t.learningRows) < t.learningLimit {
		t.learningRows = append(t.learningRows, rowData)
		return true, nil
	}

	err = t.completeLearning()
	return false, err
}

func (t *Table) completeLearning() error {
	t.learnColumnWidths()
	for _, rowData := range t.learningRows {
		if err := t.writeRow(rowData); err != nil {
			return err
		}
	}
	t.learning = false
	t.learningRows = nil
	return nil
}

func (t *Table) learnColumnWidths() {
	for i, column := range t.columns {
		if !column.learn {
			continue
		}
		learnedWidth := len(column.header)
		for j := range t.learningRows {
			actualWidth := len(t.learningRows[j][i])
			if actualWidth > learnedWidth {
				learnedWidth = actualWidth
			}
		}
		column.width = learnedWidth
	}
}

func (t *Table) writeRow(rowData []string) error {
	rowWidth := 2 * len(rowData)
	for _, column := range t.columns {
		rowWidth += column.width
	}
	var rowBuffer bytes.Buffer
	rowBuffer.Grow(rowWidth)

	for i, columnValue := range rowData {
		if i > 0 {
			rowBuffer.WriteString("  ")
		}
		actualWidth := len(columnValue)
		desiredWidth := t.columns[i].width
		switch {
		case actualWidth > desiredWidth:
			rowBuffer.WriteString(columnValue[0:desiredWidth])
		case actualWidth < desiredWidth:
			rowBuffer.WriteString(columnValue)
			for j := 0; j < desiredWidth-actualWidth; j++ {
				rowBuffer.WriteString(" ")
			}
		default:
			rowBuffer.WriteString(columnValue)
		}
	}
	rowBuffer.WriteString("\n")

	_, err := rowBuffer.WriteTo(t.printer)
	return err
}

func (t *Table) WriteHeaders() error {
	headers := make([]interface{}, len(t.columns))
	for i, column := range t.columns {
		headers[i] = column.header
	}
	return t.WriteRow(headers)
}

func (t *Table) WriteObject(data map[string]interface{}) error {
	values := make([]interface{}, len(t.columns))
	for i, column := range t.columns {
		values[i] = digValue(data, column.name)
	}
	return t.WriteRow(values)
}

func (t *Table) WriteRawObject(raw json.RawMessage) error {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	return t.WriteObject(data)
}

func (t *Table) Flush() error {
	if t.learning {
		return t.completeLearning()
	}
	return nil
}

func (t *Table) Close() error {
	return t.Flush()
}

func digValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := interface{}(data)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}
