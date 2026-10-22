package localmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DetectHost gathers the local, non-secret machine facts used by Assess. It
// deliberately does not inspect processes, credentials, or GPU drivers.
func DetectHost() Host {
	osName := runtime.GOOS
	directory, source, directoryErr := ollamaModelDirectory(osName)
	totalMemory, availableMemory, availableDisk, memoryErr, diskErr := detectHostResources(directory)
	if directoryErr != nil {
		if diskErr != "" {
			diskErr += "; "
		}
		diskErr += directoryErr.Error()
	}
	return Host{
		OS:                   osName,
		Architecture:         runtime.GOARCH,
		TotalMemoryBytes:     totalMemory,
		AvailableMemoryBytes: availableMemory,
		AvailableMemoryKnown: memoryErr == "",
		ModelDirectory:       directory,
		ModelDirectorySource: source,
		AvailableDiskBytes:   availableDisk,
		AvailableDiskKnown:   diskErr == "",
		MemoryError:          memoryErr,
		DiskError:            diskErr,
	}
}

func ollamaModelDirectory(osName string) (string, string, error) {
	if configured := strings.TrimSpace(os.Getenv("OLLAMA_MODELS")); configured != "" {
		return filepath.Clean(configured), "OLLAMA_MODELS", nil
	}
	switch osName {
	case "linux":
		// This is Ollama's documented path for its standard Linux installer.
		return "/usr/share/ollama/.ollama/models", "Ollama Linux default", nil
	case "darwin", "windows":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "Ollama user default", err
		}
		return filepath.Join(home, ".ollama", "models"), "Ollama user default", nil
	default:
		return "", "unknown", nil
	}
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
