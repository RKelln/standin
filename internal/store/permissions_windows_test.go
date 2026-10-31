//go:build windows

package store

import (
	"os"
	"path/filepath"
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
	defer r.Close()
	f, name, err := rootTemp(r, ".", 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer r.Remove(name)
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
	defer syscall.LocalFree(syscall.Handle(descriptor))
	var control uint16
	var revision uint32
	getControl := advapi.NewProc("GetSecurityDescriptorControl")
	ok, _, _ := getControl.Call(descriptor, uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
	if ok == 0 || control&0x1000 == 0 {
		t.Fatal("private ACL still inherits other users' access")
	}
	dacl, err := descriptorDACL(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	type aclHeader struct {
		Revision, Reserved     byte
		Size, Count, Reserved2 uint16
	}
	if (*aclHeader)(unsafe.Pointer(dacl)).Count != 1 {
		t.Fatal("private ACL grants more than one account")
	}
	var ace uintptr
	getAce := advapi.NewProc("GetAce")
	ok, _, _ = getAce.Call(dacl, 0, uintptr(unsafe.Pointer(&ace)))
	if ok == 0 || ace == 0 {
		t.Fatal("private ACL has no user grant")
	}
	if *(*byte)(unsafe.Pointer(ace)) != 0 {
		t.Fatal("private ACL is not a user allow entry")
	}
	if *(*uint32)(unsafe.Pointer(ace + 4)) != 0x001f01ff {
		t.Fatal("private ACL does not grant full file access")
	}
	actual, err := (*syscall.SID)(unsafe.Pointer(ace + 8)).String()
	if err != nil {
		t.Fatal("could not read private ACL user")
	}
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
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
