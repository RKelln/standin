// Package shape writes the shape file: a description of each column that
// holds no real value, for the AI to read when it writes the config.
//
// Rules: never a real value; never an exact count or length below 5. Counts
// and lengths are given as bands.
package shape

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
)

// CountBand gives a count as a band. Zero is "none".
func CountBand(n int) string {
	switch {
	case n == 0:
		return "none"
	case n < 5:
		return "under 5"
	case n <= 20:
		return "5-20"
	case n <= 100:
		return "21-100"
	}
	return "over 100"
}

// LengthBand gives a length in characters as a band.
func LengthBand(n int) string {
	switch {
	case n <= 5:
		return "1-5"
	case n <= 10:
		return "6-10"
	case n <= 20:
		return "11-20"
	case n <= 40:
		return "21-40"
	case n <= 80:
		return "41-80"
	}
	return "over 80"
}

var lengthOrder = map[string]int{"1-5": 0, "6-10": 1, "11-20": 2, "21-40": 3, "41-80": 4, "over 80": 5}

func shareBand(part, whole int) string {
	if part == 0 {
		return "none"
	}
	if part == whole {
		return "all"
	}
	if part < 5 {
		return "under 5 values"
	}
	pct := part * 100 / whole
	switch {
	case pct < 10:
		return "under 10%"
	case pct < 50:
		return "10-50%"
	}
	return "over 50%"
}

var (
	reEmail     = regexp.MustCompile(`^[A-Za-z0-9._%+'-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}$`)
	reISODate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reISOTime   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?$`)
	reSlashDate = regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{2,4}$`)
	reYMDSlash  = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`)
	reWordDate  = regexp.MustCompile(`(?i)^(?:\d{1,2} )?(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.? \d{1,2}(?:,? \d{4})?$|^\d{1,2} (?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.? \d{4}$`)
	reAmount    = regexp.MustCompile(`^[-+(]?[$€£]?\s?(?:\d{1,3}(?:,\d{3})+|\d+)\.(\d+)\)?$`)
	reMoneyInt  = regexp.MustCompile(`^[-+]?[$€£]\s?(?:\d{1,3}(?:,\d{3})+|\d+)$`)
	reDigits    = regexp.MustCompile(`^[-+]?\d+$`)
	reYesNo     = regexp.MustCompile(`(?i)^(?:yes|no|y|n|true|false)$`)
	reCapWord   = regexp.MustCompile(`^\p{Lu}[\p{Ll}'’]+(?:-\p{Lu}?[\p{Ll}'’]+)*$`)
	reLetDig    = regexp.MustCompile(`^[\pL\pN][\pL\pN._/#-]*$`)
	reWords     = regexp.MustCompile(`^[\pL'’ -]+$`)
	rePhoneFull = regexp.MustCompile(`^\+?[\d\s().-]{10,}$`)
	rePostal    = regexp.MustCompile(`(?i)^(?:[ABCEGHJ-NPRSTVXY]\d[ABCEGHJ-NPRSTV-Z][ -]?\d[ABCEGHJ-NPRSTV-Z]\d|\d{5}-\d{4}|[A-Z]{1,2}\d[A-Z\d]? ?\d[A-Z]{2})$`)
)

// Describe returns the shape string of one value. It never contains the value.
func Describe(v string) string {
	t := strings.TrimSpace(v)
	switch {
	case reEmail.MatchString(t):
		return "email-like"
	case reISODate.MatchString(t):
		return "date YYYY-MM-DD"
	case reISOTime.MatchString(t):
		return "date and time"
	case reYMDSlash.MatchString(t):
		return "date YYYY/MM/DD"
	case reSlashDate.MatchString(t):
		return "date NN/NN/YYYY"
	case reWordDate.MatchString(t):
		return "date in words"
	case reYesNo.MatchString(t):
		return "yes/no"
	}
	if m := reAmount.FindStringSubmatch(t); m != nil {
		n := len(m[1])
		if n == 1 {
			return "amount 1 decimal"
		}
		return fmt.Sprintf("amount %d decimals", n)
	}
	if reMoneyInt.MatchString(t) {
		return "amount, no decimals"
	}
	if rePhoneFull.MatchString(t) && len(scan.OnlyDigits(t)) >= 10 && len(scan.OnlyDigits(t)) <= 15 && !reDigits.MatchString(t) {
		return "phone-like"
	}
	if reDigits.MatchString(t) {
		return "digits"
	}
	if rePostal.MatchString(t) {
		return "postal-code-like"
	}
	words := strings.Fields(t)
	if len(words) >= 1 && len(words) <= 4 {
		allCap := true
		for _, w := range words {
			if !reCapWord.MatchString(w) {
				allCap = false
				break
			}
		}
		if allCap {
			switch len(words) {
			case 1:
				return "one capitalised word"
			case 2:
				return "two capitalised words"
			}
			return "capitalised words"
		}
	}
	if utf8.RuneCountInString(t) > 40 || len(words) >= 5 {
		return "free text"
	}
	if len(words) == 1 && reLetDig.MatchString(t) && hasLetter(t) && hasDigit(t) {
		return "letters and digits"
	}
	if reWords.MatchString(t) {
		return "words"
	}
	return "other text"
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

var kindOf = map[string]string{
	"email-like": "email", "date YYYY-MM-DD": "date", "date and time": "date and time",
	"date YYYY/MM/DD": "date", "date NN/NN/YYYY": "date", "date in words": "date",
	"yes/no": "yes/no", "amount, no decimals": "amount", "phone-like": "phone",
	"digits": "number or ID", "postal-code-like": "postal code",
	"one capitalised word": "name-like word", "two capitalised words": "name-like",
	"capitalised words": "name-like", "free text": "free text",
	"letters and digits": "code or ID", "words": "text", "other text": "text",
}

// Kind returns a plain kind word for a shape string.
func Kind(shape string) string {
	if strings.HasPrefix(shape, "amount ") {
		return "amount"
	}
	if k, ok := kindOf[shape]; ok {
		return k
	}
	return "text"
}

// ColumnInfo is the shape of one column.
type ColumnInfo struct {
	Index      int
	Label      string // header, or [COLUMN_n] if the header looks like data
	HeaderFlag string // why the header was hidden ("" if not)
	Kind       string
	Values     int
	Empty      int
	Distinct   int
	MinLen     int
	MaxLen     int
	AvgLen     float64
	AvgSpaces  float64
	Shapes     []ShapeCount // most common first, at most 3
	IsKey      bool
}

// ShapeCount is a shape string and how many values have it.
type ShapeCount struct {
	Shape string
	Count int
}

// Label returns how a column is named for the AI: its name, or [COLUMN_n]
// if any part of the name looks like data. flag says why.
func Label(name string, index int) (label, flag string) {
	if name == dataset.KeyColumn {
		return name, ""
	}
	if kind, ok := scan.LooksLikeDataHeader(name); ok {
		return fmt.Sprintf("[COLUMN_%d]", index+1), kind
	}
	if name == "" {
		return fmt.Sprintf("[COLUMN_%d]", index+1), "empty"
	}
	return name, ""
}

// Columns works out the shape of each column.
func Columns(d *dataset.Dataset) []ColumnInfo {
	var out []ColumnInfo
	for i, name := range d.Columns {
		ci := ColumnInfo{Index: i, IsKey: name == dataset.KeyColumn}
		ci.Label, ci.HeaderFlag = Label(name, i)
		counts := map[string]int{}
		distinct := map[string]bool{}
		totalLen, totalSpaces := 0, 0
		for _, v := range d.Values[i] {
			ci.Values++
			if strings.TrimSpace(v) == "" {
				ci.Empty++
				continue
			}
			distinct[mapping.Normalize(v)] = true
			n := utf8.RuneCountInString(v)
			if ci.MinLen == 0 || n < ci.MinLen {
				ci.MinLen = n
			}
			if n > ci.MaxLen {
				ci.MaxLen = n
			}
			totalLen += n
			totalSpaces += strings.Count(strings.TrimSpace(v), " ")
			counts[Describe(v)]++
		}
		ci.Distinct = len(distinct)
		if filled := ci.Values - ci.Empty; filled > 0 {
			ci.AvgLen = float64(totalLen) / float64(filled)
			ci.AvgSpaces = float64(totalSpaces) / float64(filled)
		}
		for s, n := range counts {
			ci.Shapes = append(ci.Shapes, ShapeCount{s, n})
		}
		sort.Slice(ci.Shapes, func(a, b int) bool {
			if ci.Shapes[a].Count != ci.Shapes[b].Count {
				return ci.Shapes[a].Count > ci.Shapes[b].Count
			}
			return ci.Shapes[a].Shape < ci.Shapes[b].Shape
		})
		if len(ci.Shapes) > 0 {
			ci.Kind = Kind(ci.Shapes[0].Shape)
		} else {
			ci.Kind = "empty"
		}
		if len(ci.Shapes) > 3 {
			ci.Shapes = ci.Shapes[:3]
		}
		if ci.IsKey {
			ci.Kind = "keys of the records (data)"
		}
		out = append(out, ci)
	}
	return out
}

// LooksFreeText reports whether a column reads like free text: long values
// on average, or many spaces.
func (c ColumnInfo) LooksFreeText() bool {
	return c.AvgLen > 40 || c.AvgSpaces >= 5
}

func lengthRange(c ColumnInfo) string {
	if c.MaxLen == 0 {
		return "no values"
	}
	lo, hi := LengthBand(c.MinLen), LengthBand(c.MaxLen)
	if lo == hi {
		return lo + " characters"
	}
	return "from " + lo + " to " + hi + " characters"
}

// Markdown builds the shape file. shownPath is the file name to show
// (already checked; "" if the name itself looks like data).
func Markdown(d *dataset.Dataset, shownPath, version string) string {
	var b strings.Builder
	title := shownPath
	if title == "" {
		title = "a raw file (name hidden)"
	}
	fmt.Fprintf(&b, "# Shape of %s\n\n", title)
	b.WriteString("Made by Standin " + version + ". This file describes the data. It holds no real values from it.\n")
	b.WriteString("Counts and lengths are given in ranges, so a single value cannot be picked out.\n\n")
	if shownPath == "" {
		b.WriteString("**The file name looks like private data** (for example a name or a number). ")
		b.WriteString("Ask the person to rename the raw file to something plain, like `invoices.csv`, before going on.\n\n")
	}
	kind := d.Kind
	if d.Kind == dataset.JSON {
		kind += ", " + d.JSONShape
	}
	fmt.Fprintf(&b, "- Kind: %s\n", kind)
	fmt.Fprintf(&b, "- Text encoding: %s\n", d.Encoding)
	rows := fmt.Sprint(d.Rows)
	if d.Rows < 5 {
		rows = CountBand(d.Rows)
	}
	label := "Rows"
	if d.Kind == dataset.JSON {
		label = "Records"
	}
	fmt.Fprintf(&b, "- %s: %s\n", label, rows)
	fmt.Fprintf(&b, "- Columns: %d\n\n", len(d.Columns))

	b.WriteString("## Columns\n\n")
	for _, c := range Columns(d) {
		fmt.Fprintf(&b, "### %d. %s\n\n", c.Index+1, mdCode(c.Label))
		writeColumn(&b, c)
	}
	b.WriteString(ConfigHelp)
	return b.String()
}

// writeColumn writes the facts about one column (no heading).
func writeColumn(b *strings.Builder, c ColumnInfo) {
	if c.HeaderFlag != "" {
		what := map[string]string{"email": "an email address", "phone": "a phone number", "digits": "a number",
			"name": "a person's name", "empty": "nothing (the header is blank)"}[c.HeaderFlag]
		if c.HeaderFlag == "empty" {
			fmt.Fprintf(b, "- Header: blank. In the config, call this column `%s`.\n", c.Label)
		} else {
			fmt.Fprintf(b, "- **Header hidden:** it looks like %s, so it may be private data. In the config, call this column `%s`. Standin will swap the header for a stand-in too.\n", what, c.Label)
		}
	}
	if c.IsKey {
		b.WriteString("- These are the names of the records in the top JSON object. They are data. Standin swaps them for `[KEY_n]` stand-ins unless the config says `keep`.\n")
	}
	fmt.Fprintf(b, "- Looks like: %s\n", c.Kind)
	fmt.Fprintf(b, "- Empty: %s\n", shareBand(c.Empty, c.Values))
	fmt.Fprintf(b, "- Different values: %s\n", CountBand(c.Distinct))
	fmt.Fprintf(b, "- Length: %s\n", lengthRange(c))
	if c.LooksFreeText() {
		b.WriteString("- Reads like free text. If the config leaves it out, Standin drops it.\n")
	}
	if len(c.Shapes) > 0 {
		b.WriteString("- Shapes of the values:\n")
		for _, s := range c.Shapes {
			fmt.Fprintf(b, "  - %s: %s\n", s.Shape, CountBand(s.Count))
		}
	}
	b.WriteString("\n")
}

func mdCode(s string) string {
	if strings.Contains(s, "`") {
		return s
	}
	return "`" + s + "`"
}

// ConfigHelp is the note at the end of every shape file.
const ConfigHelp = "## How to write the config\n\n" +
	"Write `standin.config.json` in the project folder. For each column, choose one action:\n\n" +
	"- `tokenize`: swap each value for a stand-in like `[PERSON_1]`. The same value always gets the same stand-in. Needs a `category`.\n" +
	"- `keep`: leave the real values as they are. Use it only when the work needs the real value and it is not private (amounts, dates, status words). A person must approve every keep and every scan.\n" +
	"- `drop`: blank the value. Use it for things the work does not need. It may have a `category`; Standin still swaps that value where it shows up in scan text.\n" +
	"- `scan`: free text. Standin swaps known values and patterns (emails, phone numbers, long numbers, postal codes) and leaves the rest. Names it does not know stay in, so a person must approve it too.\n\n" +
	"Categories: `PERSON ORG EMAIL PHONE ACCOUNT ID ADDRESS POSTAL KEY VALUE NAME`.\n" +
	"A column the config does not name is tokenized as `VALUE`. A free-text column it does not name is dropped.\n" +
	"Give each choice a short `reason`. Use the column names above (for a hidden header, use `[COLUMN_n]`; it can only be `tokenize` or `drop`).\n" +
	"For JSON, nested names use dots (`address.city`) and lists use `[]` (`items[].name`).\n\n" +
	"```json\n" +
	"{\n" +
	"  \"version\": 1,\n" +
	"  \"files\": [\n" +
	"    {\n" +
	"      \"match\": \"raw/invoices*.csv\",\n" +
	"      \"columns\": {\n" +
	"        \"Customer\":  {\"action\": \"tokenize\", \"category\": \"PERSON\", \"reason\": \"customer names\"},\n" +
	"        \"Amount\":    {\"action\": \"keep\", \"reason\": \"needed to match payments\"},\n" +
	"        \"Notes\":     {\"action\": \"scan\", \"reason\": \"free text; replace known names and patterns\"},\n" +
	"        \"Internal\":  {\"action\": \"drop\", \"reason\": \"not needed\"}\n" +
	"      }\n" +
	"    }\n" +
	"  ]\n" +
	"}\n" +
	"```\n\n" +
	"`match` is a pattern for file paths in the project, like `raw/*.csv` or `raw/**/*.json`. The first rule that matches a file is used.\n"

// LengthOrder is exposed for tests.
func LengthOrder(band string) int { return lengthOrder[band] }
