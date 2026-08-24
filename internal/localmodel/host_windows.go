//go:build windows

package localmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func detectHostResources(directory string) (uint64, uint64, uint64, string, string) {
	total, available, memoryErr := windowsMemory()
	disk, diskErr := windowsAvailableDisk(directory)
	return total, available, disk, memoryErr, diskErr
}

func windowsMemory() (uint64, uint64, string) {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 {
		return 0, 0, fmt.Sprintf("read system memory: %v", err)
	}
	return status.TotalPhys, status.AvailPhys, ""
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func windowsAvailableDisk(directory string) (uint64, string) {
	directory, err := windowsExistingDirectory(directory)
	if err != nil {
		return 0, err.Error()
	}
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0, fmt.Sprintf("encode Ollama model directory: %v", err)
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(path, &available, nil, nil); err != nil {
		return 0, fmt.Sprintf("stat filesystem for %s: %v", directory, err)
	}
	return available, ""
}

func windowsExistingDirectory(path string) (string, error) {
	for {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect Ollama model directory %s: %v", path, err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("no existing parent for Ollama model directory %s", path)
		}
		path = parent
	}
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}
