package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Publish relative to the held parent directory, with replacement disabled.
// Root.Rename permits replacement; a check followed by Rename would race.
func publishProfile(parent *os.Root, from, to string) error {
	if !filepath.IsLocal(from) || filepath.Base(to) != to || !filepath.IsLocal(to) {
		return errors.New("invalid profile publication path")
	}
	stage, err := parent.OpenRoot(filepath.Dir(from))
	if err != nil {
		return err
	}
	defer stage.Close()
	sourceDir, err := stage.Open(".")
	if err != nil {
		return err
	}
	defer sourceDir.Close()
	destinationDir, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer destinationDir.Close()
	name, err := windows.NewNTUnicodeString(filepath.Base(from))
	if err != nil {
		return err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(sourceDir.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE, &attributes, &windows.IO_STATUS_BLOCK{}, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	encoded, err := windows.UTF16FromString(to)
	if err != nil {
		return err
	}
	// FILE_RENAME_INFORMATION has a variable length UTF-16 name. Keep the
	// native alignment of RootDirectory on both x64 and ARM64.
	info := struct {
		ReplaceIfExists uint8
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [256]uint16
	}{RootDirectory: windows.Handle(destinationDir.Fd()), FileNameLength: uint32((len(encoded) - 1) * 2)}
	if len(encoded) > len(info.FileName) {
		return errors.New("profile name is too long")
	}
	copy(info.FileName[:], encoded)
	err = windows.NtSetInformationFile(handle, &windows.IO_STATUS_BLOCK{}, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Offsetof(info.FileName))+info.FileNameLength, windows.FileRenameInformation)
	runtime.KeepAlive(sourceDir)
	runtime.KeepAlive(destinationDir)
	return err
}
