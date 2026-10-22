package localmodel

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func detectHostResources(directory string) (uint64, uint64, uint64, string, string) {
	status := struct {
		Length, MemoryLoad                                                                                                           uint32
		TotalPhysical, AvailablePhysical, TotalPageFile, AvailablePageFile, TotalVirtual, AvailableVirtual, AvailableExtendedVirtual uint64
	}{}
	status.Length = uint32(unsafe.Sizeof(status))
	result, _, memoryErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&status)))
	memoryIssue := ""
	if result == 0 {
		memoryIssue = fmt.Sprintf("inspect system memory: %v", memoryErr)
	}
	path, err := existingDirectory(directory)
	if err != nil {
		return status.TotalPhysical, status.AvailablePhysical, 0, memoryIssue, err.Error()
	}
	encoded, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return status.TotalPhysical, status.AvailablePhysical, 0, memoryIssue, err.Error()
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(encoded, &available, &total, &free); err != nil {
		return status.TotalPhysical, status.AvailablePhysical, 0, memoryIssue, fmt.Sprintf("inspect model filesystem: %v", err)
	}
	return status.TotalPhysical, status.AvailablePhysical, available, memoryIssue, ""
}
