//go:build linux

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// runWithTTY runs standin with stdin, stdout and stderr on a new pseudo-terminal,
// typing answer into it. Stdlib only: opens /dev/ptmx with Linux ioctls.
func (p *project) runWithTTY(answer string, args ...string) (result, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return result{}, err
	}
	defer func() { _ = master.Close() }() // Early-failure cleanup; the normal path checks Close below.
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK,
		uintptr(unsafe.Pointer(&unlock))); e != 0 {
		return result{}, e
	}
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN,
		uintptr(unsafe.Pointer(&n))); e != 0 {
		return result{}, e
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return result{}, err
	}
	cmd := exec.Command(binPath, args...)
	cmd.Dir = p.dir
	cmd.Env = append(p.env(), "TERM=dumb")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		return result{}, errors.Join(err, slave.Close())
	}
	if err := slave.Close(); err != nil {
		return result{}, errors.Join(err, cmd.Process.Kill(), cmd.Wait())
	}
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&out, master)
		done <- copyErr
	}()
	if _, err := io.WriteString(master, answer); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		closeErr := master.Close()
		return result{}, errors.Join(err, killErr, waitErr, closeErr, <-done)
	}
	err = cmd.Wait()
	// A Linux PTY reports EIO after the final slave closes. Drain before closing
	// the master so we do not discard the child's last output.
	copyErr := <-done
	closeErr := master.Close()
	if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
		return result{}, errors.Join(copyErr, closeErr)
	}
	if closeErr != nil {
		return result{}, closeErr
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return result{}, err
	}
	r := result{args: args, code: code, stdout: strings.ReplaceAll(out.String(), "\r\n", "\n")}
	p.outputs = append(p.outputs, r.stdout)
	return r, nil
}
