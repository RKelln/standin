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
	defer master.Close()
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
		slave.Close()
		return result{}, err
	}
	slave.Close()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() { io.Copy(&out, master); close(done) }() // ends with EIO when the child exits
	master.Write([]byte(answer))
	err = cmd.Wait()
	master.Close()
	<-done
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
