// Package output remembers console write failures so commands cannot report
// success after printing only part of their result or instructions.
package output

import (
	"fmt"
	"io"

	"github.com/RKelln/standin/internal/fail"
)

// Writer stops writing after the first error, including a short write.
type Writer struct {
	dst io.Writer
	err error
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

// Monitor owns the checked stdout and stderr for one command. Calls may discard
// fmt's immediate return values because Finish checks these writers. Interactive
// commands must also call Error before consuming input or saving approval.
type Monitor struct {
	Stdout, Stderr *Writer
}

func New(stdout, stderr io.Writer) *Monitor {
	return &Monitor{Stdout: &Writer{dst: stdout}, Stderr: &Writer{dst: stderr}}
}

func (m *Monitor) Error() error {
	if m.Stdout.err != nil || m.Stderr.err != nil {
		return Failure()
	}
	return nil
}

// Failure is data-free and does not promise that a save has not happened.
func Failure() error {
	return fail.New("output-write", "I could not finish printing the result or instructions. Files may already have been saved. Check them before trying again.")
}

// Finish keeps the original failure private: an OS error can contain a path or
// other input. The final diagnostic is best effort because stderr may be broken.
func (m *Monitor) Finish(code int) int {
	if err := m.Error(); err != nil {
		_, _ = fmt.Fprintln(m.Stderr.dst, "Stopped: "+err.Error())
		return 2
	}
	return code
}
