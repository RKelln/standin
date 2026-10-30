package scan

import (
	"regexp"
	"strings"
	"unicode"
)

// Words often found in column headers and file names. Two capitalised words
// where one of these appears ("Customer Name") are not taken as a person's name.
var commonWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		name names first last middle full given family surname customer customers client clients
		member members vendor supplier payee payer contact company business org organisation organization
		invoice invoices bill receipt order orders payment payments date time day month year week
		due paid amount total subtotal balance price cost tax fee fees credit debit net gross qty
		quantity number no id code ref reference account acct bank card type kind status state
		notes note memo description details comment comments message subject email e-mail mail phone
		mobile cell fax address street city town province region postal zip country line unit
		start end from to sent received created updated last first item items product service
		category group tag tags currency rate method source category value key label title role
		job dept department branch store location site home work office main other extra
		monday tuesday wednesday thursday friday saturday sunday
		january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec the of and for per by in on at
		export report statement summary list data file sheet copy draft final new old`) {
		commonWords[w] = true
	}
}

var (
	capWord   = regexp.MustCompile(`^\p{Lu}[\p{Ll}'’]+(?:-\p{Lu}?[\p{Ll}'’]+)*$`)
	digitRun6 = regexp.MustCompile(`\d{6,}`)
	allDigits = regexp.MustCompile(`^[\d\s().+-]*\d{3,}[\d\s().+-]*$`)
	dateLike  = regexp.MustCompile(`^\d{4}[-/.]\d{1,2}([-/.]\d{1,2})?$|^\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}$|^\d{4}$`)
)

// LooksLikeData reports whether a header, JSON key or file name looks like a
// private value: an email, a phone number, digits, or two capitalised words
// that read like a name. kind is one of "email", "phone", "digits", "name".
func LooksLikeData(s string) (kind string, ok bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}
	if emailRe.MatchString(t) {
		return "email", true
	}
	if len(phoneFinder{phoneNA}.find(t)) > 0 || len(phoneFinder{phoneIntl}.find(t)) > 0 {
		return "phone", true
	}
	if !dateLike.MatchString(t) && (digitRun6.MatchString(t) || allDigits.MatchString(t)) {
		return "digits", true
	}
	words := strings.FieldsFunc(t, func(r rune) bool { return unicode.IsSpace(r) || r == '_' || r == ',' })
	for i := 0; i+1 < len(words); i++ {
		a, b := words[i], words[i+1]
		if capWord.MatchString(a) && capWord.MatchString(b) &&
			!commonWords[strings.ToLower(a)] && !commonWords[strings.ToLower(b)] {
			return "name", true
		}
	}
	return "", false
}

// CategoryForKind gives the stand-in category for a LooksLikeData kind.
func CategoryForKind(kind string) string {
	switch kind {
	case "email":
		return "EMAIL"
	case "phone":
		return "PHONE"
	case "digits":
		return "ID"
	case "name":
		return "PERSON"
	}
	return "VALUE"
}

// The phone patterns are plain regexps; give them the same look-around rules.
type phoneFinder struct{ re *regexp.Regexp }

func (p phoneFinder) find(s string) [][2]int {
	pat := &Pattern{re: p.re, before: isWordPlus, after: isDigit,
		check: func(m string) bool { return len(OnlyDigits(m)) >= 10 }}
	return pat.find(s)
}
