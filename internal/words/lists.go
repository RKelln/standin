// Package words finds the words in free text that might be private (names,
// mostly), and swaps them for stand-ins like [NAME_1].
//
// It uses two word lists that are built into the program (see lists/ and
// THIRD_PARTY.md): English words (SCOWL size 60, lower-case entries only) and
// first names (Ontario and Quebec baby names). The program never fetches
// them; tools/fetch_wordlists.py makes them at dev time.
package words

import (
	"bufio"
	"compress/gzip"
	"embed"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
)

//go:embed lists/english.txt.gz lists/firstnames.txt.gz
var listFiles embed.FS

var (
	loadOnce sync.Once
	errLoad  error
	english  map[string]bool
	first    map[string]bool
)

func readList(name string) (map[string]bool, error) {
	f, err := listFiles.Open("lists/" + name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // An embedded read-only handle has no writes to flush.
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }() // Scanner checks decoding errors; Close only releases the reader.
	out := map[string]bool{}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		w := sc.Text()
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		out[w] = true
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Load reads the built-in word lists once. Call it before anything else in
// this package; it fails closed if a list cannot be read.
func Load() error {
	loadOnce.Do(func() {
		var err error
		if english, err = readList("english.txt.gz"); err == nil {
			first, err = readList("firstnames.txt.gz")
		}
		if err != nil || len(english) < 1000 || len(first) < 1000 {
			errLoad = fail.New("word-lists", "My built-in word lists are damaged, so I cannot check free text. Get a fresh copy of standin.")
		}
	})
	return errLoad
}

func mustLoad() {
	if err := Load(); err != nil {
		panic(err) // callers check Load first; this is a program bug
	}
}

// InDict reports whether w is an English word (lower-case entries only),
// after taking off 's. A hyphenated word counts only if every part does.
func InDict(w string) bool {
	mustLoad()
	lw := strings.ToLower(strings.ReplaceAll(w, "’", "'"))
	switch lw {
	case "i'm", "i'd", "i'll", "i've":
		return true
	}
	if english[lw] {
		return true
	}
	if strings.HasSuffix(lw, "'s") && english[lw[:len(lw)-2]] {
		return true
	}
	if strings.Contains(lw, "-") {
		any := false
		for _, p := range strings.Split(lw, "-") {
			if p == "" {
				continue
			}
			if !InDict(p) {
				return false
			}
			any = true
		}
		return any
	}
	return false
}

// IsFirstName reports whether w (any case) is in the first-name list.
func IsFirstName(w string) bool {
	mustLoad()
	return first[Key(w)]
}

// Key is the form a word is stored and matched in: lower case, curly
// apostrophe made straight, 's taken off.
func Key(w string) string {
	lw := strings.ToLower(strings.ReplaceAll(w, "’", "'"))
	return strings.TrimSuffix(lw, "'s")
}

// Calendar words: day and month names, English and French, with short forms.
var calendar = map[string]bool{}

// Words that are never names, even in a name spot ("Hi there", "Thanks all").
var noise = map[string]bool{}

// Words just before a name ("Hi Grace", "Thanks Kofi", "called Ana").
var greet = map[string]bool{}

// Titles before a name ("Dr. Okafor", "Mme Gagnon").
var titles = map[string]bool{}

// commonName: a one-word NAME row whose word is also an English word. If the
// lists cannot load, nothing is skipped (more matching, not less).
func commonName(e mapping.Entry) bool {
	if e.Category != Category || strings.ContainsAny(e.Key, " \t") || Load() != nil {
		return false
	}
	return InDict(e.Key)
}

// headerNamed reports whether a header, table header or JSON key holds a
// word that reads like a name (decision 46): a word that is not an English
// word or a common header word, and is capitalised, in the first-name list,
// or in a script with no capitals. Short all-caps words (2 to 4 letters, like
// SKU or UTC) are taken as short forms unless they are first names. If the
// lists cannot load, every header counts (fail closed).
func headerNamed(name string) bool {
	if Load() != nil {
		return true
	}
	for _, t := range Tokens(name) {
		w := name[t[0]:t[1]]
		n := utf8.RuneCountInString(w)
		k := Key(w)
		if n < 2 || scan.IsCommonHeaderWord(k) || calendar[k] || InDict(w) {
			continue
		}
		if first[k] || IsUncased(w) {
			return true
		}
		if !IsCapitalised(w) {
			continue
		}
		if n <= 4 && strings.ToUpper(w) == w {
			continue // a short form: SKU, UTC, CRM
		}
		return true
	}
	return false
}

func init() {
	scan.SkipKnown = commonName
	scan.HeaderWords = headerNamed
	for _, w := range strings.Fields(`monday tuesday wednesday thursday friday saturday sunday
		mon tue tues wed thu thur thurs fri sat sun
		january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec
		lundi mardi mercredi jeudi vendredi samedi dimanche janvier février mars avril mai juin
		juillet août septembre octobre novembre décembre`) {
		calendar[w] = true
	}
	for _, w := range strings.Fields(`there all team everyone folks you again so for the a an much in
		to our your my this it and i we tout le la les vous toi monsieur madame re fwd fw sir madam`) {
		noise[w] = true
	}
	for _, w := range strings.Fields(`hi hello hey dear thanks thank cheers from regards bonjour salut
		merci allo allô cc with per by called told met asked emailed texted`) {
		greet[w] = true
	}
	// Titles and ranks: a title opens a name spot ("Mr Price") and is never
	// itself a candidate. Ranks are here because "Sergeant Gamble" and
	// "Flight Lieutenant Winks" are how the police and services are named in
	// records (decision 91). Ambiguous words are left out on purpose: "Major"
	// and "Chief" are as often ordinary ("a major issue", "Chief Roadworks").
	for _, w := range strings.Fields(`mr mrs ms mx dr mme mlle m prof
		lord lady sir dame judge justice lieutenant sergeant constable inspector
		superintendent captain colonel corporal admiral advocate`) {
		titles[w] = true
	}
}

// Notices returns the source and licence notes that head each built-in
// list (its "# " lines), for `standin licences`.
func Notices() (string, error) {
	var b strings.Builder
	for _, name := range []string{"english.txt.gz", "firstnames.txt.gz"} {
		f, err := listFiles.Open("lists/" + name)
		if err != nil {
			return "", err
		}
		zr, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close() // Preserve the decode failure; the embedded file is read-only.
			return "", err
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			l := sc.Text()
			if !strings.HasPrefix(l, "#") {
				break
			}
			b.WriteString(strings.TrimPrefix(strings.TrimPrefix(l, "#"), " ") + "\n")
		}
		if err := errors.Join(sc.Err(), zr.Close(), f.Close()); err != nil {
			return "", err
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// IsCalendar reports whether w is a day or month name.
func IsCalendar(w string) bool { return calendar[Key(w)] }
