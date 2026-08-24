//go:build darwin

package localmodel

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func detectHostResources(directory string) (uint64, uint64, uint64, string, string) {
	total, available, memoryErr := darwinMemory()
	disk, diskErr := darwinAvailableDisk(directory)
	return total, available, disk, memoryErr, diskErr
}

func darwinMemory() (uint64, uint64, string) {
	output, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, fmt.Sprintf("read hw.memsize: %v", err)
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil || total == 0 {
		return 0, 0, "hw.memsize was not a positive byte count"
	}
	// macOS has no stable equivalent to Linux MemAvailable. Total physical
	// memory remains a reliable capacity check; Doctor reports this caveat.
	return total, 0, "macOS available-memory measurement is not implemented"
}

func darwinAvailableDisk(directory string) (uint64, string) {
	directory, err := darwinExistingDirectory(directory)
	if err != nil {
		return 0, err.Error()
	}
	var statistics unix.Statfs_t
	if err := unix.Statfs(directory, &statistics); err != nil {
		return 0, fmt.Sprintf("stat filesystem for %s: %v", directory, err)
	}
	return uint64(statistics.Bavail) * uint64(statistics.Bsize), ""
}

func darwinExistingDirectory(path string) (string, error) {
	for {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect Ollama model directory %s: %v", path, err)
		}
		parent := strings.TrimRight(path, "/")
		if parent == "" {
			return "", fmt.Errorf("no existing parent for Ollama model directory %s", path)
		}
		parent = parent[:strings.LastIndex(parent, "/")+1]
		if parent == path {
			return "", fmt.Errorf("no existing parent for Ollama model directory %s", path)
		}
		path = parent
	}
}
