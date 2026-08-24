//go:build linux

package localmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func detectHostResources(directory string) (uint64, uint64, uint64, string, string) {
	total, available, memoryErr := linuxMemory()
	disk, diskErr := unixAvailableDisk(directory)
	return total, available, disk, memoryErr, diskErr
}

func linuxMemory() (uint64, uint64, string) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Sprintf("read /proc/meminfo: %v", err)
	}
	values := make(map[string]uint64)
	present := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ":") {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		values[name] = value * 1024
		present[name] = true
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if !present["MemTotal"] || total == 0 {
		return 0, available, "MemTotal was not present in /proc/meminfo"
	}
	if !present["MemAvailable"] {
		return total, 0, "MemAvailable was not present in /proc/meminfo"
	}
	return total, available, ""
}

func unixAvailableDisk(directory string) (uint64, string) {
	directory, err := existingDirectory(directory)
	if err != nil {
		return 0, err.Error()
	}
	var statistics unix.Statfs_t
	if err := unix.Statfs(directory, &statistics); err != nil {
		return 0, fmt.Sprintf("stat filesystem for %s: %v", directory, err)
	}
	return uint64(statistics.Bavail) * uint64(statistics.Bsize), ""
}

func existingDirectory(path string) (string, error) {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return "", fmt.Errorf("Ollama model directory is unknown")
	}
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
