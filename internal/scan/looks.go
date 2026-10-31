package scan

import (
	"regexp"
	"strings"
	"unicode"
)

// Words often found in column headers and file names. Two capitalised words
// where both are on this list ("Customer Name") are not taken as a person's
// name. The list is long on purpose: a header taken for a name is hidden as
// [COLUMN_n], and then the config cannot keep it.
var commonWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		name names first last middle full given family surname customer customers client clients
		member members vendor vendors supplier suppliers payee payer contact contacts company business
		org organisation organization owner staff employee employees user users person people
		invoice invoices bill billing receipt receipts order orders payment payments purchase purchases
		sale sales refund refunds deposit deposits withdrawal transfer transaction transactions
		date time day days month months year years week weeks period quarter
		due paid unpaid open closed overdue pending amount amounts total totals subtotal balance price
		prices cost costs tax taxes fee fees credit credits debit debits net gross qty quantity
		discount shipping handling freight sku upc unit units each per rate rates percent
		number num no id code codes ref reference references account accounts acct bank card cards
		transit institution routing branch iban swift type kind status state
		notes note memo memos description details detail comment comments message messages subject
		email emails e-mail mail phone phones mobile cell fax address addresses street city town
		province region postal zip country line lines suite apt zone area
		start end from to sent received created updated modified last first item items product
		products service services category categories group groups tag tags currency method methods
		source value values key keys label title role job dept department store location locations site
		home work office main other extra misc miscellaneous general
		gst hst pst vat usd cad eur gbp
		monday tuesday wednesday thursday friday saturday sunday
		january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec the of and for per by in on at a an or is
		export report statement statements summary list data file files sheet copy draft final new old
		yes active inactive count average avg min max sum
		ledger journal payroll inventory budget expense expenses income revenue donor donors
		donation donations volunteer volunteers backup csv tsv json txt`) {
		commonWords[w] = true
	}
}

var (
	nameWord  = regexp.MustCompile(`^(?:O['’]|D['’]|Mc|Mac)?\p{Lu}[\p{Ll}'’]+$`)
	capsWord  = regexp.MustCompile(`^\p{Lu}{2,}(?:['’]\p{Lu}+)?$`)
	lowerWord = regexp.MustCompile(`^\p{Ll}{2,}$`)
	digitRun6 = regexp.MustCompile(`\d{6,}`)
	allDigits = regexp.MustCompile(`^[\d\s().+-]*\d{3,}[\d\s().+-]*$`)
	dateLike  = regexp.MustCompile(`^\d{4}[-/.]\d{1,2}([-/.]\d{1,2})?$|^\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}$|^\d{4}$`)
)

// isName is a capitalised word, allowing O'Brien, McNab and Mei-Ling.
func isName(w string) bool {
	for _, part := range strings.Split(w, "-") {
		if !nameWord.MatchString(part) {
			return false
		}
	}
	return true
}

func isCommon(w string) bool { return commonWords[strings.ToLower(w)] }

// IsCommonHeaderWord reports whether w is on the list of words often found
// in column headers and labels (name, date, amount ...).
func IsCommonHeaderWord(w string) bool { return isCommon(w) }

// LooksLikeData reports whether a header, JSON key or file name looks like a
// private value: an email, a phone number, digits, or two words that read
// like a name. kind is one of "email", "phone", "digits", "name".
//
// Names: two capitalised words (Zelda Quorn, Sean O'Brien, Ann McNab), two
// words in capitals (ZELDA QUORN), or two lower-case words (lee_pip), unless
// both are common header words. A common word followed by a capitalised
// word that is not common (Bill Gatesby) is also taken as a name.
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
	words := strings.FieldsFunc(t, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.Is(unicode.Zs, r) || r == '_' || r == ',' || r == '.'
	})
	for i := 0; i+1 < len(words); i++ {
		a, b := words[i], words[i+1]
		ca, cb := isCommon(a), isCommon(b)
		switch {
		case isName(a) && isName(b) && !cb:
			// Both not common, or a first name that is also a word (Bill, Mark).
			return "name", true
		case capsWord.MatchString(a) && capsWord.MatchString(b) && !ca && !cb:
			return "name", true
		case lowerWord.MatchString(a) && lowerWord.MatchString(b) && !ca && !cb:
			return "name", true
		}
	}
	return "", false
}

// LooksLikeDataName checks a header or JSON path: the whole name, then each
// dot-separated part (with any "[]" removed). Shape and scrub both use it, so
// a header hidden in one is hidden in the other.
func LooksLikeDataName(name string) (kind string, ok bool) {
	if kind, ok := LooksLikeData(name); ok {
		return kind, true
	}
	for _, p := range strings.Split(name, ".") {
		if kind, ok := LooksLikeData(strings.TrimSuffix(p, "[]")); ok {
			return kind, true
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
