package privatefs

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRejectsBroadWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := MkdirAll(path); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err == nil {
		t.Fatal("accepted Everyone access")
	}
	if err := Protect(path); err != nil {
		t.Fatal(err)
	}
}
