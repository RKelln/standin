package output

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type shortWriter struct{ calls int }

func (w *shortWriter) Write(p []byte) (int, error) {
	w.calls++
	return len(p) - 1, nil
}

func TestShortWriteStopsLaterOutputAndFailsExit(t *testing.T) {
	dst := &shortWriter{}
	var stderr bytes.Buffer
	m := New(dst, &stderr)
	if _, err := m.Stdout.Write([]byte("first")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write was accepted: %v", err)
	}
	if _, err := m.Stdout.Write([]byte("second")); !errors.Is(err, io.ErrShortWrite) || dst.calls != 1 {
		t.Fatalf("retried a broken output: %v, %d writes", err, dst.calls)
	}
	if m.Finish(0) != 2 || !strings.Contains(stderr.String(), "Files may already have been saved") {
		t.Fatalf("write failure reported success: %s", stderr.String())
	}
}
