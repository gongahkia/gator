package localmodel

import (
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
		ModelDirectory:       directory,
		ModelDirectorySource: source,
		AvailableDiskBytes:   availableDisk,
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
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "Ollama macOS default", err
		}
		return filepath.Join(home, ".ollama", "models"), "Ollama macOS default", nil
	case "windows":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "Ollama Windows default", err
		}
		return filepath.Join(home, ".ollama", "models"), "Ollama Windows default", nil
	default:
		return "", "unknown", nil
	}
}
