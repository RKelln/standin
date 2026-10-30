// Package dataset reads the files Standin supports (CSV, TSV, JSON) into
// columns of values, and writes them back with values changed.
//
// Anything it is not sure about, it refuses. It never puts a value from the
// file into an error message.
package dataset

import (
	"bytes"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/textenc"
)

// KeyColumn is the column name for the keys of an object of records.
const KeyColumn = "(key)"

// Kinds of file.
const (
	CSV  = "CSV"
	TSV  = "TSV"
	JSON = "JSON"
)

// RefuseHint is what to do with a file Standin cannot read.
const RefuseHint = "Save it as CSV, or remove private details by hand."

// Dataset is a parsed file.
type Dataset struct {
	Kind      string
	Encoding  string
	JSONShape string     // "array of records" or "object of records"
	Columns   []string   // header names, or JSON paths like address.city
	Values    [][]string // per column, every value ("" when empty)
	Recs      [][]int    // per column, the row or record number (from 1) of each value
	Rows      int

	comma   rune
	header  []string
	records [][]string
	root    *jnode
	colOf   map[string]int
}

// KindFor returns the kind for a file name, or a refusal.
func KindFor(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return CSV, nil
	case ".tsv":
		return TSV, nil
	case ".json":
		return JSON, nil
	case ".pdf":
		return "", fail.New("unsupported", "I cannot read PDF files. "+RefuseHint)
	case ".xlsx", ".xls", ".xlsm", ".ods", ".numbers":
		return "", fail.New("unsupported", "I cannot read spreadsheet files like this one yet. "+RefuseHint)
	case ".docx", ".doc", ".odt", ".rtf", ".pages":
		return "", fail.New("unsupported", "I cannot read word processor files. "+RefuseHint)
	case ".png", ".jpg", ".jpeg", ".gif", ".heic", ".webp", ".tif", ".tiff", ".bmp":
		return "", fail.New("unsupported", "I cannot read images. "+RefuseHint)
	case ".txt":
		return "", fail.New("unsupported", "I do not know if this .txt file is a table. If it is, rename it to .csv (commas) or .tsv (tabs).")
	}
	return "", fail.New("unsupported", "I cannot read this kind of file. "+RefuseHint)
}

// Load reads and parses a file.
func Load(path string) (*Dataset, error) {
	kind, err := KindFor(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fail.New("read", "I could not read the file (%s).", fail.Reason(err))
	}
	return Parse(kind, b)
}

// Parse parses file bytes of the given kind.
func Parse(kind string, b []byte) (*Dataset, error) {
	if what, ok := textenc.Sniff(b); ok {
		return nil, fail.New("unsupported", "This file is really %s, not %s. %s", what, kind, RefuseHint)
	}
	text, enc, err := textenc.Decode(b)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fail.New("empty", "This file is empty.")
	}
	if (kind == CSV || kind == TSV) && hasLoneCR(text) {
		return nil, fail.New("lone-cr", "This file ends its lines in an old way (a lone carriage return, from old Mac programs), so I cannot tell where rows end. Open it in a text editor or spreadsheet and save it with normal line endings, then try again.")
	}
	var d *Dataset
	switch kind {
	case CSV:
		d, err = parseCSV(text, ',')
	case TSV:
		d, err = parseCSV(text, '\t')
	case JSON:
		d, err = parseJSON(text)
	default:
		return nil, fail.New("unsupported", "I cannot read this kind of file. "+RefuseHint)
	}
	if err != nil {
		return nil, err
	}
	d.Kind, d.Encoding = kind, enc
	return d, nil
}

func parseCSV(text string, comma rune) (*Dataset, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = comma
	r.FieldsPerRecord = 0
	recs, err := r.ReadAll()
	if err != nil {
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			switch {
			case errors.Is(pe.Err, csv.ErrFieldCount):
				return nil, fail.New("ragged", "Line %d has a different number of fields than the header line. Every row needs the same number of fields. Fix that line, then try again.", pe.Line)
			case errors.Is(pe.Err, csv.ErrQuote), errors.Is(pe.Err, csv.ErrBareQuote):
				return nil, fail.New("bad-quote", "Line %d has a quote mark (\") where I do not expect one. Fix that line, then try again.", pe.StartLine)
			}
			return nil, fail.New("bad-csv", "I could not read line %d of this file.", pe.StartLine)
		}
		return nil, fail.New("bad-csv", "I could not read this file as a table.")
	}
	if len(recs) == 0 {
		return nil, fail.New("empty", "This file is empty.")
	}
	d := &Dataset{comma: comma, header: recs[0], records: recs[1:], Rows: len(recs) - 1}
	d.Columns = append([]string(nil), recs[0]...)
	d.Values = make([][]string, len(d.Columns))
	d.Recs = make([][]int, len(d.Columns))
	for r, rec := range d.records {
		for i, v := range rec {
			d.Values[i] = append(d.Values[i], v)
			d.Recs[i] = append(d.Recs[i], r+1)
		}
	}
	return d, nil
}

// Rewriter says how to change a file.
type Rewriter struct {
	// Cell gets a column index and a value ("" for empty or null). It returns
	// the new value, and changed=false to leave the original as it was.
	Cell func(col int, v string) (out string, changed bool)
	// Name gets a header cell or JSON key (not an object-of-records key) and
	// returns its new text, or the same text.
	Name func(name string) string
}

// Write returns the file with the rewriter applied, as UTF-8.
func (d *Dataset) Write(rw Rewriter) ([]byte, error) {
	if d.Kind == JSON {
		return d.writeJSON(rw)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = d.comma
	h := make([]string, len(d.header))
	for i, name := range d.header {
		h[i] = rw.Name(name)
	}
	if err := w.Write(h); err != nil {
		return nil, fail.New("write", "I could not build the new file.")
	}
	for _, rec := range d.records {
		out := make([]string, len(rec))
		for i, v := range rec {
			nv, changed := rw.Cell(i, v)
			if changed {
				out[i] = nv
			} else {
				out[i] = v
			}
		}
		if err := w.Write(out); err != nil {
			return nil, fail.New("write", "I could not build the new file.")
		}
	}
	w.Flush()
	if w.Error() != nil {
		return nil, fail.New("write", "I could not build the new file.")
	}
	return buf.Bytes(), nil
}

// Where tells a person where value number i of column col is, such as
// "row 3" (row 1 is the first row after the header) or "record 3".
func (d *Dataset) Where(col, i int) string {
	if d.Kind == JSON {
		return "record " + strconv.Itoa(d.Recs[col][i])
	}
	return "row " + strconv.Itoa(d.Recs[col][i])
}

// Header returns the CSV header cells (nil for JSON).
func (d *Dataset) Header() []string { return d.header }

// hasLoneCR reports a carriage return that is not followed by a line feed.
func hasLoneCR(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && (i+1 == len(s) || s[i+1] != '\n') {
			return true
		}
	}
	return false
}
