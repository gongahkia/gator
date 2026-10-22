package privatefs

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protect replaces inherited access with an inheritable DACL for this user,
// SYSTEM, and administrators. Never apply it to a selected workspace or a
// shared parent; only Gator-owned directories and files belong here.
func Protect(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("private path must be an ordinary file or directory")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	inherit := ""
	if info.IsDir() {
		inherit = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;%s;FA;;;%s)(A;%s;FA;;;SY)(A;%s;FA;;;BA)", inherit, user.User.Sid.String(), inherit, inherit))
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return fmt.Errorf("protect private Windows path: %w", err)
	}
	return Check(path)
}

func Check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("private path is not ordinary")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	allowed := func(sid *windows.SID) bool {
		return sid.Equals(user.User.Sid) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
	}
	if owner == nil || !allowed(owner) {
		return errors.New("private path has an unexpected owner")
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if acl == nil {
		return errors.New("private path has an unrestricted DACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !allowed((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return errors.New("private path grants access to another Windows account")
		}
	}
	runtime.KeepAlive(descriptor)
	return nil
}

// FlushFileBuffers does not support the read-only directory handles returned
// by os.Open. Files are flushed before rename; Windows has no equivalent to
// Unix directory fsync through this API. Do not hide file flush errors.
func SyncDir(*os.File) error { return nil }
