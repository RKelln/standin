// Package config reads and checks standin.config.json: the list of rules that
// says, for each column, whether to tokenize, keep, drop or scan it.
//
// The config holds no secrets. The AI may read and write it. A person must
// approve it before any "keep" takes effect.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
)

// FileName is the config file name in the project folder.
const FileName = "standin.config.json"

// Actions.
const (
	Tokenize = "tokenize"
	Keep     = "keep"
	Drop     = "drop"
	Scan     = "scan"
	Words    = "words" // text: the word check (a kind of scan; needs approval)
)

// Groups of words the config may keep in free text. Only built-in groups:
// the config never names a word, since the AI never sees one.
var KeepGroups = []string{"calendar"}

// Column is the rule for one column.
type Column struct {
	Action   string `json:"action"`
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Text is the rule for the free text in a .md or .txt file.
type Text struct {
	Action     string   `json:"action"` // words or drop
	KeepGroups []string `json:"keep_groups,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// File is the rule set for files that match a glob.
type File struct {
	Match   string            `json:"match"`
	Columns map[string]Column `json:"columns,omitempty"`
	// For .md and .txt files: "Label: value" lines, Markdown table columns
	// (by header, in every table), and the rest of the text.
	Fields      map[string]Column `json:"fields,omitempty"`
	Tables      map[string]Column `json:"tables,omitempty"`
	Text        *Text             `json:"text,omitempty"`
	order       []string          // column names in the order written
	fieldOrder  []string
	tablesOrder []string
}

// KeepsGroup reports whether the text rule keeps a built-in word group.
func (f *File) KeepsGroup(g string) bool {
	if f == nil || f.Text == nil || f.Text.Action != Words {
		return false
	}
	for _, k := range f.Text.Groups() {
		if k == g {
			return true
		}
	}
	return false
}

// Groups are the word groups the text rule keeps. With no "keep_groups"
// the calendar group is kept: day and month names are almost never private,
// and forgetting the line turns "Friday 10am" into [NAME_7]. An empty list
// ("keep_groups": []) keeps nothing.
func (t *Text) Groups() []string {
	if t.KeepGroups == nil {
		return []string{"calendar"}
	}
	return t.KeepGroups
}

// UsesWords reports whether the rule asks for the word check on its text.
func (f *File) UsesWords() bool { return f != nil && f.Text != nil && f.Text.Action == Words }

// Config is the whole file.
type Config struct {
	Version int    `json:"version"`
	Files   []File `json:"files"`
	Hash    string `json:"-"` // sha256 of the file bytes, hex
}

// Load reads the config. exists is false if there is no config file.
func Load(project string) (c *Config, exists bool, err error) {
	b, err := os.ReadFile(path.Join(toSlash(project), FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fail.New("config-read", "I could not read %s (%s).", FileName, fail.Reason(err))
	}
	c, err = Parse(b)
	return c, true, err
}

func toSlash(p string) string { return strings.ReplaceAll(p, "\\", "/") }

// HashBytes is the hash an approval stores.
func HashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func bad(format string, args ...any) error {
	return fail.New("config-bad", FileName+": "+format+" Fix it, then try again.", args...)
}

// Parse reads and checks a config.
func Parse(b []byte) (*Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		msg := err.Error()
		if strings.Contains(msg, `unknown field "keep_groups"`) {
			return nil, bad(`"keep_groups" goes inside "text": {"action": "words", "keep_groups": [...]}, not at the top of a file rule.`)
		}
		if strings.Contains(msg, "unknown field") {
			return nil, bad("it has a name I do not know (%s).", strings.TrimPrefix(msg, "json: "))
		}
		return nil, bad("it is not valid JSON in the shape I expect (%s).", strings.TrimPrefix(msg, "json: "))
	}
	if dec.More() {
		return nil, bad("it has extra text after the end.")
	}
	if c.Version != 1 {
		return nil, bad(`"version" must be 1.`)
	}
	orders, err := columnOrder(b)
	if err != nil {
		return nil, err
	}
	for i := range c.Files {
		f := &c.Files[i]
		if f.Match == "" {
			return nil, bad(`file rule %d has no "match".`, i+1)
		}
		if err := checkGlob(f.Match); err != nil {
			return nil, bad(`file rule %d has a "match" pattern I cannot read.`, i+1)
		}
		if i < len(orders) {
			f.order, f.fieldOrder, f.tablesOrder = orders[i][0], orders[i][1], orders[i][2]
		}
		for name, col := range f.Columns {
			if err := checkColumn(name, col); err != nil {
				return nil, bad("in %q, column %q: %s", f.Match, name, err.Error())
			}
		}
		for what, cols := range map[string]map[string]Column{"field": f.Fields, "table column": f.Tables} {
			for name, col := range cols {
				if err := checkTextColumn(name, col); err != nil {
					return nil, bad("in %q, %s %q: %s", f.Match, what, name, err.Error())
				}
			}
		}
		if t := f.Text; t != nil {
			switch t.Action {
			case Words:
				for _, g := range t.KeepGroups {
					if !validGroup(g) {
						return nil, bad(`in %q, "keep_groups" can only hold %s. It cannot name words: the person chooses words in standin approve.`, f.Match, strings.Join(KeepGroups, ", "))
					}
				}
			case Drop:
				if len(t.KeepGroups) > 0 {
					return nil, bad(`in %q, "keep_groups" only goes with "text": {"action": "words"}.`, f.Match)
				}
			case "":
				return nil, bad(`in %q, "text" has no "action" (words or drop).`, f.Match)
			default:
				return nil, bad(`in %q, the "text" action must be words or drop.`, f.Match)
			}
		}
	}
	c.Hash = HashBytes(b)
	return &c, nil
}

func checkColumn(name string, col Column) error {
	switch col.Action {
	case Tokenize:
		if col.Category == "" {
			return errors.New(`"tokenize" needs a "category" (one of ` + strings.Join(mapping.Categories, ", ") + `).`)
		}
		if !mapping.ValidCategory(col.Category) {
			return errors.New(`"category" must be one of ` + strings.Join(mapping.Categories, ", ") + `.`)
		}
		if name == "(key)" && col.Category != "KEY" {
			return errors.New(`the keys of an object of records use category "KEY".`)
		}
	case Drop:
		if col.Category != "" && !mapping.ValidCategory(col.Category) {
			return errors.New(`"category" must be one of ` + strings.Join(mapping.Categories, ", ") + `.`)
		}
		if name == "(key)" {
			return errors.New(`the keys of an object of records can only be "keep" or "tokenize".`)
		}
	case Keep, Scan:
		if col.Category != "" {
			return errors.New(`"category" only goes with "tokenize" or "drop".`)
		}
		if name == "(key)" && col.Action != Keep {
			return errors.New(`the keys of an object of records can only be "keep" or "tokenize".`)
		}
		if _, ok := ColumnRef(name); ok {
			return errors.New(`a column named by position ([COLUMN_n]) can only be "tokenize" or "drop", because the position could change in the next export. Rename the header in the raw file if you need to keep it.`)
		}
	case "":
		return errors.New(`it has no "action" (tokenize, keep, drop or scan).`)
	default:
		return errors.New(`"action" must be tokenize, keep, drop or scan.`)
	}
	return nil
}

func validGroup(g string) bool {
	for _, k := range KeepGroups {
		if g == k {
			return true
		}
	}
	return false
}

// checkTextColumn checks a field or table column rule. The same actions as
// a column, but no [COLUMN_n] and no (key).
func checkTextColumn(name string, col Column) error {
	if _, ok := ColumnRef(name); ok || name == "(key)" {
		return errors.New(`use the name shown in the shape file.`)
	}
	return checkColumn(name, col)
}

// columnOrder gets the column, field and table names of each file rule in
// the order written, so review prints them in the same order as the file.
func columnOrder(b []byte) ([][3][]string, error) {
	var raw struct {
		Files []struct {
			Columns json.RawMessage `json:"columns"`
			Fields  json.RawMessage `json:"fields"`
			Tables  json.RawMessage `json:"tables"`
		} `json:"files"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, bad("it is not valid JSON.")
	}
	var out [][3][]string
	for _, f := range raw.Files {
		var o [3][]string
		for i, part := range []json.RawMessage{f.Columns, f.Fields, f.Tables} {
			names, err := keyOrder(part)
			if err != nil {
				return nil, err
			}
			o[i] = names
		}
		out = append(out, o)
	}
	return out, nil
}

func keyOrder(part json.RawMessage) ([]string, error) {
	{
		var names []string
		if len(part) > 0 && string(part) != "null" {
			dec := json.NewDecoder(bytes.NewReader(part))
			if _, err := dec.Token(); err != nil {
				return nil, bad("it is not valid JSON.")
			}
			seen := map[string]bool{}
			for dec.More() {
				t, err := dec.Token()
				if err != nil {
					return nil, bad("it is not valid JSON.")
				}
				k, _ := t.(string)
				if seen[k] {
					return nil, bad("%q is listed twice in one file rule.", k)
				}
				seen[k] = true
				names = append(names, k)
				var skip json.RawMessage
				if err := dec.Decode(&skip); err != nil {
					return nil, bad("it is not valid JSON.")
				}
			}
		}
		return names, nil
	}
}

// ColumnNames returns the rule's column names in the order written.
func (f *File) ColumnNames() []string { return f.order }

// FieldNames returns the rule's field names in the order written.
func (f *File) FieldNames() []string { return f.fieldOrder }

// TableNames returns the rule's table column names in the order written.
func (f *File) TableNames() []string { return f.tablesOrder }

func checkGlob(p string) error {
	for _, seg := range strings.Split(p, "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}

// Match reports whether a project-relative slash path matches a glob. Rules
// are path.Match per segment, plus "**" for any number of folders.
func Match(pattern, name string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegs(p, n []string) bool {
	for len(p) > 0 {
		if p[0] == "**" {
			for i := 0; i <= len(n); i++ {
				if matchSegs(p[1:], n[i:]) {
					return true
				}
			}
			return false
		}
		if len(n) == 0 {
			return false
		}
		ok, err := path.Match(p[0], n[0])
		if err != nil || !ok {
			return false
		}
		p, n = p[1:], n[1:]
	}
	return len(n) == 0
}

// RuleFor returns the first file rule whose match fits rel, or nil.
func (c *Config) RuleFor(rel string) *File {
	if c == nil {
		return nil
	}
	for i := range c.Files {
		if Match(c.Files[i].Match, rel) {
			return &c.Files[i]
		}
	}
	return nil
}

var columnRef = regexp.MustCompile(`^\[COLUMN_([1-9][0-9]*)\]$`)

// ColumnRef returns n if name is "[COLUMN_n]".
func ColumnRef(name string) (int, bool) {
	m := columnRef.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// ColumnFor finds the rule for a column by its name or by "[COLUMN_n]"
// (n counts from 1). If both are given, that is a refusal: it is not clear
// which one to use.
func (f *File) ColumnFor(name string, index int) (Column, bool, error) {
	byName, okName := f.Columns[name]
	byRef, okRef := f.Columns["[COLUMN_"+strconv.Itoa(index+1)+"]"]
	if okName && okRef && name != "[COLUMN_"+strconv.Itoa(index+1)+"]" {
		return Column{}, false, fail.New("config-bad", "%s: column %d is listed twice, by name and as [COLUMN_%d]. Keep one. Fix it, then try again.", FileName, index+1, index+1)
	}
	if okName {
		return byName, true, nil
	}
	if okRef {
		return byRef, true, nil
	}
	return Column{}, false, nil
}
