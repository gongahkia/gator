package runtime

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func IsWSL() bool {
	return isWSL(runtime.GOOS, os.ReadFile)
}

func isWSL(goos string, readFile func(string) ([]byte, error)) bool {
	if goos != "linux" {
		return false
	}
	for _, path := range []string{"/proc/sys/kernel/osrelease", "/proc/version"} {
		data, err := readFile(path)
		if err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft") {
			return true
		}
	}
	return false
}

func ValidateWSL() error {
	return validateWSL(runtime.GOOS, os.ReadFile)
}

func validateWSL(goos string, readFile func(string) ([]byte, error)) error {
	if goos != "linux" {
		return errors.New("WSL requires a Linux runtime")
	}
	if !isWSL(goos, readFile) {
		return fmt.Errorf("WSL environment not detected")
	}
	return nil
}
