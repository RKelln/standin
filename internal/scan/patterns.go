// Package scan finds private details in free text: values already in the
// mapping, and patterns such as emails, phone numbers and account numbers.
//
// The patterns are ported from Get2Know's share-safely check_share.py (same
// author, MIT licence; https://github.com/RKelln/get2know). That script notes
// its sources: the Luhn card and SIN checks and the Canadian postal code
// letters follow Microsoft Presidio's recognizers (MIT); the street address
// and PO box shapes are adapted from CommonRegex (MIT).
//
// Go's regexp is RE2: it has no look-behind, look-ahead or back-references.
// Those checks are done in code here (see okBefore and okAfter), and the SIN
// pattern spells out its three separator forms instead of using \1.
package scan

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pattern is one kind of private detail.
type Pattern struct {
	Kind     string // plain words, printed by check
	Category string // stand-in category used by scrub
	re       *regexp.Regexp
	group    int               // submatch to use (0 = whole match)
	before   func(r rune) bool // the rune before must NOT be this (nil = no rule)
	after    func(r rune) bool // the rune after must NOT be this (nil = no rule)
	check    func(m string) bool
}

func isDigit(r rune) bool         { return r >= '0' && r <= '9' }
func isDigitDash(r rune) bool     { return isDigit(r) || r == '-' }
func isWordPlus(r rune) bool      { return isWordASCII(r) || r == '+' }
func isWordASCII(r rune) bool     { return r == '_' || isDigit(r) || (r|0x20 >= 'a' && r|0x20 <= 'z') }
func isLongNumBefore(r rune) bool { return isDigit(r) || strings.ContainsRune(".,$-", r) }
func isLongNumAfter(r rune) bool  { return isDigit(r) || strings.ContainsRune(".,-", r) }

// LuhnOK reports whether a digit string passes the Luhn checksum.
func LuhnOK(digits string) bool {
	total := 0
	for i := 0; i < len(digits); i++ {
		n := int(digits[len(digits)-1-i] - '0')
		if i%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		total += n
	}
	return len(digits) > 0 && total%10 == 0
}

// OnlyDigits keeps the digits of s.
func OnlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if isDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var usStates = "AL|AK|AZ|AR|CA|CO|CT|DE|FL|GA|HI|ID|IL|IN|IA|KS|KY|LA|ME|MD|MA|MI|MN|MS|MO|" +
	"MT|NE|NV|NH|NJ|NM|NY|NC|ND|OH|OK|OR|PA|RI|SC|SD|TN|TX|UT|VT|VA|WA|WV|WI|WY|DC"

var streetWords = "street|st|avenue|ave|road|rd|drive|dr|boulevard|blvd|lane|ln|court|ct|crescent|cres|" +
	"place|pl|way|terrace|trail|trl|parkway|pkwy|highway|hwy|circle|cir|square|sq|" +
	"gardens|gdns|grove|close|row|line|sideroad|concession"

// Email is used on its own by the shape checks.
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+'-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)

// A handle is a username typed with an @, such as @grace_lee or @jdoe. The word
// check cannot see one: "grace" and "lee" are English words, and a word after
// "@" is left alone on purpose. So a handle is claimed whole, like an email.
var handleRe = regexp.MustCompile(`@[A-Za-z0-9_][A-Za-z0-9_.-]{1,29}[A-Za-z0-9_]`)

// atWords are the words after an @ that are not handles: CSS at-rules and
// decorator keywords. This is a closed vocabulary from a specification, not a
// list of people, so it cannot grow, and it keeps a pasted style sheet from
// turning into usernames.
var atWords = map[string]bool{
	"media": true, "import": true, "keyframes": true, "font-face": true, "supports": true,
	"layer": true, "container": true, "charset": true, "page": true, "property": true,
	"scope": true, "apply": true, "override": true, "deprecated": true, "suppress": true,
	"namespace": true, "starting-style": true, "tailwind": true,
}

var phoneNA = regexp.MustCompile(`(?:\+?1[ .-]?)?\(?[2-9]\d{2}\)?[ .-]?\d{3}[ .-]?\d{4}`)
var phoneIntl = regexp.MustCompile(`\+\d{1,3}(?:[ .-]?\(?\d{1,4}\)?){2,5}`)

// Patterns, in the order they claim text. An earlier pattern wins an overlap.
var Patterns = []*Pattern{
	{Kind: kindEmail, Category: categoryEmail, re: emailRe},
	{Kind: kindHandle, Category: categoryUser, re: handleRe,
		before: isWordPlus,
		check: func(m string) bool {
			// A handle holds at least one letter ("@123" is a number), and is
			// not a CSS at-rule or a decorator keyword.
			if atWords[strings.ToLower(m[1:])] {
				return false
			}
			return strings.IndexFunc(m, unicode.IsLetter) >= 0
		}},
	{Kind: "card number", Category: categoryAccount,
		re:     regexp.MustCompile(`\d(?:[ -]?\d){12,18}`),
		before: isDigitDash, after: isDigit,
		check: func(m string) bool { return LuhnOK(OnlyDigits(m)) }},
	// SIN: 9 digits, first not 0 or 8, all the same separator (none, space or dash).
	{Kind: "SIN (social insurance number)", Category: "ID",
		re:     regexp.MustCompile(`[1-79]\d{2} \d{3} \d{3}|[1-79]\d{2}-\d{3}-\d{3}|[1-79]\d{8}`),
		before: isDigitDash, after: isDigitDash,
		check: func(m string) bool { return LuhnOK(OnlyDigits(m)) }},
	{Kind: "phone number", Category: categoryPhone, re: phoneNA,
		before: isWordPlus, after: isDigit,
		check: func(m string) bool { return len(OnlyDigits(m)) >= 10 }},
	{Kind: "phone number", Category: categoryPhone, re: phoneIntl,
		before: isWordPlus, after: isDigit,
		check: func(m string) bool { return len(OnlyDigits(m)) >= 10 }},
	{Kind: "bank account (IBAN)", Category: categoryAccount,
		re: regexp.MustCompile(`\b[A-Z]{2}\d{2}(?: ?[A-Z0-9]{4}){2,7}(?: ?[A-Z0-9]{1,4})?\b`)},
	{Kind: "account number", Category: categoryAccount, group: 1,
		re: regexp.MustCompile(`(?i)\b(?:account|acct|a/c|transit|institution|routing|iban|member|policy|licen[cs]e|` +
			`customer|client)\s*(?:no\.?|number|#|num)?\s*[:#]?\s*([A-Z]{0,4}[\d][\d -]{3,}\d)`)},
	{Kind: "long number (account?)", Category: categoryAccount,
		re:     regexp.MustCompile(`\d{8,}`),
		before: isLongNumBefore, after: isLongNumAfter},
	{Kind: "postal code", Category: categoryPostal,
		re: regexp.MustCompile(`(?i)\b[ABCEGHJ-NPRSTVXY]\d[ABCEGHJ-NPRSTV-Z][ -]?\d[ABCEGHJ-NPRSTV-Z]\d\b`)},
	{Kind: "ZIP code", Category: categoryPostal,
		re: regexp.MustCompile(`\b(?:(?:` + usStates + `)\s+\d{5}(?:-\d{4})?|\d{5}-\d{4})\b`)},
	{Kind: "postcode", Category: categoryPostal,
		re: regexp.MustCompile(`\b[A-Z]{1,2}\d[A-Z\d]? ?\d[ABD-HJLNP-UW-Z]{2}\b`)},
	{Kind: kindStreetAddress, Category: categoryAddress,
		re: regexp.MustCompile(`\b\d{1,5}[A-Za-z]?(?:-\d{1,5})?,?\s+(?:[A-Z0-9][A-Za-z0-9'.-]*\s+){1,4}(?i:` + streetWords + `)\b\.?` +
			`(?:\s+(?i:north|south|east|west)\b|\s+[NSEW]\b)?`)},
	{Kind: kindStreetAddress, Category: categoryAddress,
		re: regexp.MustCompile(`(?i)\b\d{1,5},?\s+(?:rue|avenue|boulevard|chemin|route)\s+[A-Za-zÀ-ÿ'-]+`)},
	{Kind: kindStreetAddress, Category: categoryAddress,
		re: regexp.MustCompile(`(?i)\bP\.?\s?O\.?\s?Box\s+\d+`)},
	{Kind: kindStreetAddress, Category: categoryAddress,
		re: regexp.MustCompile(`(?i)\b(?:unit|suite|apt|apartment)\.?\s*#?\s*\d+[A-Za-z]?\b`)},
	{Kind: "password or key", Category: "VALUE",
		re: regexp.MustCompile(`(?i)\b(?:password|passwd|passcode|pin|api[ _-]?key|secret|token|login)\b\s*(?:is|:|=)\s*\S+`)},
}

// Hit is a place in text that holds a private detail.
type Hit struct {
	Start, End int
	Kind       string
	Category   string
	Standin    string // set for a known value from the mapping
}

func runeBefore(s string, i int) (rune, bool) {
	if i <= 0 {
		return 0, false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r, true
}

func runeAfter(s string, i int) (rune, bool) {
	if i >= len(s) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r, true
}

// find returns the matches of p in s, with the look-around rules applied.
// When a match fails a rule, the search goes on one rune later, as the
// original look-behind pattern would.
func (p *Pattern) find(s string) [][2]int {
	var out [][2]int
	pos := 0
	for pos <= len(s) {
		loc := p.re.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		ms, me := loc[0]+pos, loc[1]+pos
		gs, ge := loc[2*p.group]+pos, loc[2*p.group+1]+pos
		ok := loc[2*p.group] >= 0
		if ok && p.before != nil {
			if r, has := runeBefore(s, ms); has && p.before(r) {
				ok = false
			}
		}
		if ok && p.after != nil {
			if r, has := runeAfter(s, me); has && p.after(r) {
				ok = false
			}
		}
		if ok && p.check != nil && !p.check(s[gs:ge]) {
			ok = false
		}
		if ok {
			out = append(out, [2]int{gs, ge})
			if me > ms {
				pos = me
			} else {
				pos = me + 1
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(s[ms:])
		if size == 0 {
			size = 1
		}
		pos = ms + size
	}
	return out
}

// FindPatterns returns pattern hits in s that do not overlap taken spans or each other.
func FindPatterns(s string, taken []Hit) []Hit {
	var hits []Hit
	free := func(a, b int) bool {
		for _, t := range taken {
			if a < t.End && b > t.Start {
				return false
			}
		}
		for _, t := range hits {
			if a < t.End && b > t.Start {
				return false
			}
		}
		return true
	}
	for _, p := range Patterns {
		for _, loc := range p.find(s) {
			a, b := loc[0], loc[1]
			// Trim spaces, as the original does with .strip().
			for a < b && s[a] == ' ' {
				a++
			}
			for b > a && s[b-1] == ' ' {
				b--
			}
			if a < b && free(a, b) && !insideStandin(s, a, b) {
				hits = append(hits, Hit{Start: a, End: b, Kind: p.Kind, Category: p.Category})
			}
		}
	}
	return hits
}

// insideStandin reports whether [a,b) overlaps a stand-in already in the text.
func insideStandin(s string, a, b int) bool {
	for _, loc := range standinRe.FindAllStringIndex(s, -1) {
		if a < loc[1] && b > loc[0] {
			return true
		}
	}
	return false
}

var standinRe = regexp.MustCompile(`\[[A-Z]+_[1-9][0-9]*\]`)

// InsideStandin reports whether s[a:b] overlaps a stand-in already in s.
func InsideStandin(s string, a, b int) bool { return insideStandin(s, a, b) }

// HasStandin reports whether s already holds stand-in-shaped text.
func HasStandin(s string) bool { return standinRe.MatchString(s) }

// IsWordRune is a letter, digit or underscore in any script.
func IsWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
