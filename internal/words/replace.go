package words

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
)

// Category is the stand-in category for a swapped word.
const Category = "NAME"

// Replace swaps, in one line of free text: known values and patterns (as
// scan does), then every word whose key swap says yes to. A word is swapped
// where it is capitalised or in a script with no capitals, and also in lower
// case when it is not an English word ("thx oksana" is caught; "rich" and
// "grace period" stay). A trailing 's stays outside the stand-in. In a word
// joined by hyphens ("oksana-vantreeck"), each part swap says yes to is
// swapped too. keepCal: the file's rule keeps day and month names.
func Replace(line string, known *scan.Known, m *mapping.Mapping, swap func(key string) bool, keepCal bool) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	hits := Hits(line, known, keepCal)
	var b strings.Builder
	last := 0
	ok := func(w string) bool {
		return utf8.RuneCountInString(w) >= 2 && (IsCapitalised(w) || IsUncased(w) || !InDict(w)) && swap(Key(w))
	}
	var words func(seg string)
	words = func(seg string) {
		pos := 0
		for _, t := range Tokens(seg) {
			w := seg[t[0]:t[1]]
			if !ok(w) {
				if strings.Contains(w, "-") {
					b.WriteString(seg[pos:t[0]])
					parts := strings.Split(w, "-")
					for j, p := range parts {
						if j > 0 {
							b.WriteString("-")
						}
						if p != "" && ok(p) {
							words(p)
						} else {
							b.WriteString(p)
						}
					}
					pos = t[1]
				}
				continue
			}
			base := w
			if lw := strings.ToLower(strings.ReplaceAll(w, "’", "'")); strings.HasSuffix(lw, "'s") {
				_, size := utf8.DecodeLastRuneInString(w[:len(w)-1])
				base = w[:len(w)-1-size]
			}
			b.WriteString(seg[pos:t[0]])
			b.WriteString(m.Get(Category, base))
			pos = t[0] + len(base)
		}
		b.WriteString(seg[pos:])
	}
	for _, h := range hits {
		words(line[last:h.Start])
		st := h.Standin
		if st == "" {
			st = m.Get(h.Category, line[h.Start:h.End])
		}
		b.WriteString(st)
		last = h.End
	}
	words(line[last:])
	return b.String()
}

// Choices are the person's word decisions for a project: words to keep as
// they are, and words to swap. They live in words.json in the private folder.
// The swap list is mostly names, so the file is as secret as the mapping.
type Choices struct {
	Keep map[string]bool
	Swap map[string]bool
}

type choicesFile struct {
	Version int      `json:"version"`
	Note    string   `json:"note"`
	Keep    []string `json:"keep"`
	Swap    []string `json:"swap"`
}

// NewChoices is an empty set of decisions.
func NewChoices() *Choices { return &Choices{Keep: map[string]bool{}, Swap: map[string]bool{}} }

// LoadChoices reads words.json. A missing file gives empty choices and exists=false.
func LoadChoices(path string) (ch *Choices, raw []byte, exists bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewChoices(), nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fail.New("words-read", "I could not read the word choices file (%s).", fail.Reason(err))
	}
	var f choicesFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil || f.Version != 1 {
		return nil, nil, true, fail.New("words-bad", "The word choices file (words.json, in the private folder) is damaged. Fix it, or delete it and run `standin approve` again.")
	}
	ch = NewChoices()
	for _, w := range f.Keep {
		ch.Keep[Key(w)] = true
	}
	for _, w := range f.Swap {
		if k := Key(w); !ch.Keep[k] {
			ch.Swap[k] = true
		}
	}
	return ch, b, true, nil
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Marshal writes the choices as JSON a person can read and edit.
func (ch *Choices) Marshal() []byte {
	f := choicesFile{
		Version: 1,
		Note: "SECRET: the swap list holds names. Never give this file to an AI. " +
			"Words in keep stay as they are in the text the AI reads; words in swap become stand-ins like [NAME_1]. " +
			"If you change this file, run standin approve again.",
		Keep: sorted(ch.Keep),
		Swap: sorted(ch.Swap),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(f)
	return buf.Bytes()
}
