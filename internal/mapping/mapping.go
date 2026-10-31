// Package mapping keeps the secret table: each stand-in and the real value it
// takes the place of.
//
// A stand-in looks like [PERSON_1]. The same category plus normalised value
// always gets the same stand-in. The table keeps every spelling it has seen
// for a stand-in, first one first; restore uses the first.
package mapping

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/jsonsafe"
	"github.com/RKelln/standin/internal/store"
)

// Categories are the stand-in categories Standin knows.
// NAME is a word from free text (a .md or .txt file) that the word check swapped.
var Categories = []string{"PERSON", "ORG", "EMAIL", "PHONE", "ACCOUNT", "ID", "ADDRESS", "POSTAL", "KEY", "VALUE", "NAME"}

// ValidCategory reports whether c is a known category.
func ValidCategory(c string) bool {
	for _, k := range Categories {
		if k == c {
			return true
		}
	}
	return false
}

// StandinPattern matches a stand-in, such as [PERSON_12].
var StandinPattern = regexp.MustCompile(`\[([A-Z]+)_([1-9][0-9]*)\]`)

// Entry is one row of the table.
type Entry struct {
	Standin   string   `json:"standin"`
	Category  string   `json:"category"`
	Key       string   `json:"key"`
	Spellings []string `json:"spellings"`
	// Kept is set on a NAME word the person later chose to keep. Its stand-in
	// still restores, but scans no longer look for it.
	Kept bool `json:"kept,omitempty"`
}

// Value is the spelling restore puts back: the first one seen.
func (e Entry) Value() string { return e.Spellings[0] }

type file struct {
	Version int     `json:"version"`
	Note    string  `json:"note"`
	Entries []Entry `json:"standins"`
}

// Mapping is the table in memory.
type Mapping struct {
	Entries   []Entry
	byKey     map[string]int
	byStandin map[string]int
	next      map[string]int
	added     int
}

// New returns an empty table.
func New() *Mapping {
	return &Mapping{byKey: map[string]int{}, byStandin: map[string]int{}, next: map[string]int{}}
}

// Load reads the table from path. A missing file gives an empty table and exists=false.
func Load(path string) (m *Mapping, exists bool, err error) {
	if err := store.CheckReady(filepath.Dir(path)); err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), false, nil
	}
	if err != nil {
		return nil, false, fail.New("mapping-read", "I could not read the mapping file (%s): %s", path, fail.Reason(err))
	}
	m, err = Parse(b)
	return m, true, err
}

// Parse reads a table from JSON and checks it.
func Parse(b []byte) (*Mapping, error) {
	if jsonsafe.Validate(b) != nil {
		return nil, fail.New("mapping-bad", "The mapping file is damaged, so I stopped. Restore it from a backup.")
	}
	var f file
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fail.New("mapping-bad", "The mapping file is damaged, so I stopped. Restore it from a backup.")
	}
	if f.Version != 1 {
		return nil, fail.New("mapping-bad", "The mapping file has a version I do not know (%d).", f.Version)
	}
	m := New()
	counts := map[string]int{}
	for _, e := range f.Entries {
		sm := StandinPattern.FindStringSubmatch(e.Standin)
		if sm == nil || sm[0] != e.Standin || sm[1] != e.Category || !ValidCategory(e.Category) || len(e.Spellings) == 0 {
			return nil, fail.New("mapping-bad", "The mapping file is damaged (a bad row), so I stopped.")
		}
		if _, dup := m.byStandin[e.Standin]; dup {
			return nil, fail.New("mapping-bad", "The mapping file is damaged (a stand-in is listed twice), so I stopped.")
		}
		k := e.Category + "\x00" + e.Key
		if _, dup := m.byKey[k]; dup {
			return nil, fail.New("mapping-bad", "The mapping file is damaged (a value is listed twice), so I stopped.")
		}
		n, err := strconv.Atoi(sm[2])
		if err != nil || n <= 0 || n > len(f.Entries) || (e.Kept && e.Category != "NAME") {
			return nil, fail.New("mapping-bad", "The mapping file is damaged (a bad row), so I stopped.")
		}
		for _, spelling := range e.Spellings {
			if strings.TrimSpace(spelling) == "" || valueKey(spelling) != e.Key {
				return nil, fail.New("mapping-bad", "The mapping file is damaged (a value does not match its key), so I stopped.")
			}
		}
		if n > m.next[e.Category] {
			m.next[e.Category] = n
		}
		m.byKey[k] = len(m.Entries)
		m.byStandin[e.Standin] = len(m.Entries)
		m.Entries = append(m.Entries, e)
		counts[e.Category]++
	}
	for category, count := range counts {
		if m.next[category] != count {
			return nil, fail.New("mapping-bad", "The mapping file is damaged (some stand-ins are missing), so I stopped. Restore it from a backup.")
		}
	}
	return m, nil
}

// Marshal writes the table as JSON a person can read.
func (m *Mapping) Marshal() []byte {
	f := file{
		Version: 1,
		Note:    "SECRET. This file links each stand-in to the real value. Never give it to an AI or put it in a shared folder.",
		Entries: m.Entries,
	}
	if f.Entries == nil {
		f.Entries = []Entry{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(f)
	return buf.Bytes()
}

// Normalize gives the key used to match values: lower case, trimmed, inner
// spaces collapsed, trailing . , ; : removed.
func Normalize(s string) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	s = strings.TrimRight(s, ".,;: ")
	s = strings.ToLower(s)
	return s
}

// Get returns the stand-in for value in category, adding one if needed.
// The caller must not pass an empty value.
func (m *Mapping) Get(category, value string) string {
	key := valueKey(value)
	k := category + "\x00" + key
	if i, ok := m.byKey[k]; ok {
		e := &m.Entries[i]
		seen := false
		for _, s := range e.Spellings {
			if s == value {
				seen = true
				break
			}
		}
		if !seen {
			e.Spellings = append(e.Spellings, value)
		}
		return e.Standin
	}
	m.next[category]++
	st := "[" + category + "_" + strconv.Itoa(m.next[category]) + "]"
	m.byKey[k] = len(m.Entries)
	m.byStandin[st] = len(m.Entries)
	m.Entries = append(m.Entries, Entry{Standin: st, Category: category, Key: key, Spellings: []string{value}})
	m.added++
	return st
}

func valueKey(value string) string {
	key := Normalize(value)
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(value))
	}
	return key
}

// Lookup finds the row for a stand-in.
func (m *Mapping) Lookup(standin string) (Entry, bool) {
	i, ok := m.byStandin[standin]
	if !ok {
		return Entry{}, false
	}
	return m.Entries[i], true
}

// Added is how many new stand-ins were made since the table was loaded.
func (m *Mapping) Added() int { return m.added }

// CountByCategory returns how many stand-ins each category has, sorted by category.
func (m *Mapping) CountByCategory() []struct {
	Category string
	Count    int
} {
	counts := map[string]int{}
	for _, e := range m.Entries {
		counts[e.Category]++
	}
	var out []struct {
		Category string
		Count    int
	}
	for c, n := range counts {
		out = append(out, struct {
			Category string
			Count    int
		}{c, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// SetKept marks each NAME row whose word is kept (keep says yes), and
// unmarks the others.
func (m *Mapping) SetKept(keep func(key string) bool) {
	for i := range m.Entries {
		if m.Entries[i].Category == "NAME" {
			m.Entries[i].Kept = keep(m.Entries[i].Key)
		}
	}
}

// FindValue finds a row whose normalised value equals value's, in any
// category (the earliest row wins). Kept words are not found.
func (m *Mapping) FindValue(value string) (Entry, bool) {
	key := Normalize(value)
	best := -1
	for _, c := range Categories {
		if i, ok := m.byKey[c+"\x00"+key]; ok && !m.Entries[i].Kept && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return Entry{}, false
	}
	return m.Entries[best], true
}
