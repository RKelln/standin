// Command standin lets an AI work on your files without seeing your private data.
//
// Run `standin help` for the steps and commands.
package main

import (
	"os"

	"github.com/RKelln/standin/internal/cli"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var version, commit string

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		StdinIsTerminal:  cli.StdinIsTerminal,
		StdoutIsTerminal: cli.StdoutIsTerminal,
		StderrIsTerminal: cli.StderrIsTerminal,
		Getenv:           os.Getenv,
		Version:          version,
		Commit:           commit,
	}))
}
