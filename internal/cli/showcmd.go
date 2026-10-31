package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/RKelln/standin/internal/fail"
)

// A stand-in typed by the person, with or without the square brackets.
var showArgRe = regexp.MustCompile(`^\[?([A-Za-z]+_[1-9][0-9]*)\]?$`)

// runShow prints the real value behind each stand-in, for the person at a
// screen only (decision 35). Like `restore -`, it refuses a pipe, a file or
// an AI tool, so real values do not land in a transcript.
func runShow(c *ctx) error {
	if len(c.args) == 0 {
		return fail.New("usage", "Name the stand-ins to look up. For example: standin show PERSON_9")
	}
	if !c.env.StdoutIsTerminal() {
		return fail.New("stdout-not-terminal", "show prints real values, so it only prints to a screen, not to a pipe, a file or an AI tool. Run it yourself in a terminal.")
	}
	var names []string
	for _, a := range c.args {
		m := showArgRe.FindStringSubmatch(strings.TrimSpace(a))
		if m == nil {
			return fail.New("usage", "%q is not a stand-in. A stand-in looks like PERSON_9 or [PERSON_9].", a)
		}
		names = append(names, "["+strings.ToUpper(m[1])+"]")
	}
	mp, _, err := c.loadMapping(true)
	if err != nil {
		return err
	}
	for _, n := range names {
		e, ok := mp.Lookup(n)
		if !ok {
			c.warn("%s is not in the mapping. The AI may have made it up.", n)
			continue
		}
		more := ""
		if k := len(e.Spellings) - 1; k > 0 {
			more = fmt.Sprintf("   (also written %d other way(s))", k)
		}
		fmt.Fprintf(c.env.Stdout, "%s  %s%s\n", n, e.Value(), more)
	}
	return nil
}
