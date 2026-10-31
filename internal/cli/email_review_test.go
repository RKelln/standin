package cli

import (
	"strings"
	"testing"
)

func TestEmailRefusalsHideHeaderDataAndWriteNoBatch(t *testing.T) {
	// Every message and name here is invented. Refusal details can reach an
	// agent, so even invalid encoding labels must be treated as private data.
	for name, mail := range map[string]string{
		"transfer": "Subject: Notice\nContent-Transfer-Encoding: ZeldaQuorn\n\nHello.\n",
		"charset":  "Subject: Notice\nContent-Type: text/plain; charset=ZeldaQuorn\n\nHello.\n",
		"header":   "Subject: =?ZeldaQuorn?Q?Notice?=\n\nHello.\n",
		"nested": "Subject: Notice\nContent-Type: multipart/mixed; boundary=part\n\n" +
			"--part\nContent-Type: text/plain; charset=ZeldaQuorn\n\nHello.\n--part--\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, command := range []string{"shape", "scrub"} {
				p := newTP(t)
				p.write("standin.config.json", `{"version":1,"files":[]}`)
				p.write("raw/a.csv", "Amount\n10.00\n")
				p.write("raw/mail.eml", mail)
				r := p.run("", false, command)
				if r.code != 2 {
					t.Fatalf("%s must refuse invalid email: %+v", command, r)
				}
				if strings.Contains(strings.ToLower(r.stdout+r.stderr), "zeldaquorn") {
					t.Errorf("%s exposed an untrusted header: %+v", command, r)
				}
				if p.exists("shape") || p.exists("scrubbed") {
					t.Errorf("%s published output from a refused batch", command)
				}
			}
		})
	}
}
