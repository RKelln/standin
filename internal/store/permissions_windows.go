//go:build windows

package store

import (
	"errors"
	"os"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/RKelln/standin/internal/fail"
)

var advapi = syscall.NewLazyDLL("advapi32.dll")
var convertDescriptor = advapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
var setPathSecurity = advapi.NewProc("SetNamedSecurityInfoW")
var setHandleSecurity = advapi.NewProc("SetSecurityInfo")
var getDescriptorDACL = advapi.NewProc("GetSecurityDescriptorDacl")

// Unix mode bits do not set Windows ACLs. A protected DACL grants only this
// process user's SID access; directory grants propagate to newly created files.
// Administrators can still take ownership, as with Unix root.
func securePath(path string, dir bool) error {
	return withDescriptor(dir, func(descriptor uintptr) error {
		name, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			return permissionError()
		}
		dacl, err := descriptorDACL(descriptor)
		if err != nil {
			return err
		}
		const daclAndProtect = 0x00000004 | 0x80000000
		const fileObject = 1
		code, _, _ := setPathSecurity.Call(uintptr(unsafe.Pointer(name)), fileObject, daclAndProtect, 0, 0, dacl, 0)
		if code != 0 {
			return permissionError()
		}
		return nil
	})
}

func descriptorDACL(descriptor uintptr) (uintptr, error) {
	var present, defaulted uint32
	var dacl uintptr
	ok, _, _ := getDescriptorDACL.Call(descriptor, uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)))
	// A missing/null DACL grants everyone access; never pass one to Windows.
	if ok == 0 || present == 0 || dacl == 0 {
		return 0, permissionError()
	}
	return dacl, nil
}

func withDescriptor(dir bool, apply func(uintptr) error) error {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return permissionError()
	}
	defer func() { _ = token.Close() }() // Query-only token; closing cannot change the ACL result.
	user, err := token.GetTokenUser()
	if err != nil {
		return permissionError()
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return permissionError()
	}
	flags := ""
	if dir {
		flags = "OICI"
	}
	sddl, err := syscall.UTF16PtrFromString("D:P(A;" + flags + ";FA;;;" + sid + ")")
	if err != nil {
		return permissionError()
	}
	var descriptor uintptr
	ok, _, _ := convertDescriptor.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return permissionError()
	}
	defer func() { _, _ = syscall.LocalFree(syscall.Handle(descriptor)) }() // Only frees the local descriptor buffer.
	return apply(descriptor)
}

// Open an ACL/metadata handle and verify it names the already pinned, empty
// temp file. Set its ACL before any private bytes are written. WRITE_DAC alone
// cannot read the attributes that File.Stat needs for the identity check.
func secureFile(path string, f *os.File) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return filePermissionError("read the temporary file name", err)
	}
	const writeDACL = 0x00040000
	const readControl = 0x00020000
	const readAttributes = 0x00000080
	const readExtendedAttributes = 0x00000008
	const openReparsePoint = 0x00200000
	// Match Go's metadata access set for os.Root.OpenFile/File.Stat, plus the
	// right to set the DACL. See internal/syscall/windows/at_windows.go in Go
	// (https://go.dev/src/internal/syscall/windows/at_windows.go), "Allow File.Stat".
	// This handle never needs file-data read/write rights.
	const access = writeDACL | readControl | readAttributes | readExtendedAttributes
	h, err := syscall.CreateFile(name, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, openReparsePoint, 0)
	if err != nil {
		return filePermissionError("open the file permission handle", err)
	}
	check := os.NewFile(uintptr(h), path)
	defer func() { _ = check.Close() }() // Metadata/ACL handle only; private bytes use f and its checked close.
	a, err := f.Stat()
	if err != nil {
		return filePermissionError("check the open temporary file", err)
	}
	b, err := check.Stat()
	if err != nil {
		return filePermissionError("read file details for the permission check", err)
	}
	if !os.SameFile(a, b) {
		return filePermissionError("match the permission handle to the temporary file", nil)
	}
	return withDescriptor(false, func(descriptor uintptr) error {
		dacl, err := descriptorDACL(descriptor)
		if err != nil {
			return err
		}
		const daclAndProtect = 0x00000004 | 0x80000000
		const fileObject = 1
		code, _, _ := setHandleSecurity.Call(uintptr(h), fileObject, daclAndProtect, 0, 0, dacl, 0)
		if code != 0 {
			return filePermissionError("set the temporary file's access list", syscall.Errno(code))
		}
		return nil
	})
}

// step is a fixed label from secureFile, never a path or file value. Numeric
// Windows errors help diagnose a failed stage without printing private paths.
func filePermissionError(step string, err error) error {
	code := ""
	var errno syscall.Errno
	if errors.As(err, &errno) {
		code = " (Windows code " + strconv.FormatUint(uint64(errno), 10) + ")"
	}
	return fail.New("private-permissions", "I could not %s%s. I stopped before writing private data to this file. Choose a folder you own, then try again.", step, code)
}

func permissionError() error {
	return fail.New("private-permissions", "I could not limit this private folder or file to your Windows account. I stopped to keep the mapping safe. Choose a folder you own.")
}

// Windows does not support portable directory Sync through os.File. File
// contents are synced, and process interruption is detected by the marker;
// power-loss behavior still needs native filesystem testing.
func syncRoot(_ *os.Root, _ ...string) error { return nil }
