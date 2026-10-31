package words

import (
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
)

func TestEncodedRunsNeverOverlapWords(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{"order-abcdefghijkl1", "order|abcdefghijkl1"},
		{"abcdefghijkl1-order", "abcdefghijkl1|order"},
		{"order-abcdefghijkl1-tail", "order|abcdefghijkl1|tail"},
		{"order_abcdefghijkl1_tail", "order|abcdefghijkl1|tail"},
		{"order-1abcdefghijkl-tail", "order|1abcdefghijkl|tail"},
		{"order-abcdef+ghijkl-tail", "order|abcdef+ghijkl|tail"},
		{"order-abcdefghijkl1-mnopqrstuvwx2-tail", "order|abcdefghijkl1|mnopqrstuvwx2|tail"},
		{"order-éabcdefghijkl1é-tail", "order-é|abcdefghijkl1|é-tail"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			got := make([]string, 0, len(strings.Split(tc.want, "|")))
			last := 0
			for _, sp := range Tokens(tc.line) {
				if sp[0] < last || sp[1] <= sp[0] || sp[1] > len(tc.line) {
					t.Fatalf("invalid/overlapping spans: %v", Tokens(tc.line))
				}
				last = sp[1]
				got = append(got, tc.line[sp[0]:sp[1]])
			}
			if strings.Join(got, "|") != tc.want {
				t.Fatalf("tokens %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceEncodedRunPreservesSurroundingSlug(t *testing.T) {
	for _, line := range []string{
		"order-abcdefghijkl1-tail", "order_abcdefghijkl1_tail",
		"/order/abcdefghijkl1/tail", "order-1abcdefghijkl-tail", "order-abcdef+ghijkl-tail",
	} {
		m := mapping.New()
		got := Replace(line, nil, m, func(k string) bool { return len(encodedRuns(k)) != 0 }, false)
		run := encodedRuns(line)[0]
		want := line[:run[0]] + "[NAME_1]" + line[run[1]:]
		if got != want {
			t.Errorf("replace %q = %q, want %q", line, got, want)
		}
	}
}
