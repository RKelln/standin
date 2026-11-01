//go:build windows

package store

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsPrivateACLBeforeWriting(t *testing.T) {
	dir := canonicalTempDir(t)
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	assertUserOnlyACL(t, dir)
	r, err := openBoundary(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	f, name, err := rootTemp(r, ".", 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
		if err := r.Remove(name); err != nil {
			t.Error(err)
		}
	})
	info, err := f.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatal("private temp must still be empty")
	}
	assertUserOnlyACL(t, filepath.Join(r.Name(), name))
	if _, err := f.Write([]byte("invented private value")); err != nil {
		t.Fatal(err)
	}
}

func assertUserOnlyACL(t *testing.T, path string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor uintptr
	getNamed := advapi.NewProc("GetNamedSecurityInfoW")
	code, _, _ := getNamed.Call(uintptr(unsafe.Pointer(name)), 1, 4, 0, 0, 0, 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 || descriptor == 0 {
		t.Fatal("could not read private ACL")
	}
	defer func() { _, _ = syscall.LocalFree(syscall.Handle(descriptor)) }() // Only frees the native descriptor buffer.
	var control uint16
	var revision uint32
	getControl := advapi.NewProc("GetSecurityDescriptorControl")
	ok, _, _ := getControl.Call(descriptor, uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
	if ok == 0 || control&0x1000 == 0 {
		t.Fatal("private ACL still inherits other users' access")
	}
	// Receive native addresses as pointers. Keeping them in uintptr variables
	// and converting back later loses the pointer provenance required by vet.
	var dacl unsafe.Pointer
	var present, defaulted uint32
	ok, _, _ = getDescriptorDACL.Call(descriptor, uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)))
	if ok == 0 || present == 0 || dacl == nil {
		t.Fatal("private ACL has no access list")
	}
	type aclHeader struct {
		Revision, Reserved     byte
		Size, Count, Reserved2 uint16
	}
	if (*aclHeader)(dacl).Count != 1 {
		t.Fatal("private ACL grants more than one account")
	}
	var ace unsafe.Pointer
	getAce := advapi.NewProc("GetAce")
	ok, _, _ = getAce.Call(uintptr(dacl), 0, uintptr(unsafe.Pointer(&ace)))
	if ok == 0 || ace == nil {
		t.Fatal("private ACL has no user grant")
	}
	if *(*byte)(ace) != 0 {
		t.Fatal("private ACL is not a user allow entry")
	}
	if *(*uint32)(unsafe.Add(ace, 4)) != 0x001f01ff {
		t.Fatal("private ACL does not grant full file access")
	}
	actual, err := (*syscall.SID)(unsafe.Add(ace, 8)).String()
	if err != nil {
		t.Fatal("could not read private ACL user")
	}
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = token.Close() }() // Query-only handle; no access-list changes are buffered.
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	want, err := user.User.Sid.String()
	if err != nil {
		t.Fatal(err)
	}
	if actual != want {
		t.Fatal("private ACL grants a different Windows account")
	}
}

// Existing files from an older binary must not retain inherited broad access.
func TestWindowsEnsureTightensOldPrivateFiles(t *testing.T) {
	dir := canonicalTempDir(t)
	p := filepath.Join(dir, MappingFile)
	if err := os.WriteFile(p, []byte("invented old mapping"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	assertUserOnlyACL(t, p)
}

// The permission handle must be checked against the already-open temp file,
// rather than trusting whichever file the supplied path currently names.
func TestWindowsSecureFileRefusesDifferentFile(t *testing.T) {
	dir := canonicalTempDir(t)
	first := filepath.Join(dir, "first.tmp")
	other := filepath.Join(dir, "other.tmp")
	f, err := os.OpenFile(first, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(other, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := secureFile(other, f); err == nil {
		t.Fatal("permission check accepted a path to a different file")
	} else if !strings.Contains(err.Error(), "match the permission handle") {
		t.Fatalf("identity check did not reach the handle comparison: %v", err)
	}
	for _, path := range []string{first, other} {
		st, err := os.Stat(path)
		if err != nil || st.Size() != 0 {
			t.Fatal("permission check must not write file contents")
		}
	}
	// A matching handle still gets a protected current-user ACL while empty.
	if err := secureFile(first, f); err != nil {
		t.Fatalf("could not secure matching empty file: %v", err)
	}
	assertUserOnlyACL(t, first)
}

func TestWindowsPermissionFailureDetailsHidePaths(t *testing.T) {
	err := filePermissionError("read file details for the permission check", &os.PathError{
		Op: "GetFileInformationByHandle", Path: `C:\Zelda Quorn\private.tmp`, Err: syscall.ERROR_ACCESS_DENIED,
	})
	if !strings.Contains(err.Error(), "Windows code 5") || strings.Contains(err.Error(), "Zelda") || strings.Contains(err.Error(), "private.tmp") {
		t.Fatalf("permission failure lost its safe detail: %v", err)
	}
}
