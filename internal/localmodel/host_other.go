//go:build !linux && !darwin && !windows

package localmodel

import "fmt"

func detectHostResources(string) (uint64, uint64, uint64, string, string) {
	return 0, 0, 0, fmt.Sprintf("host-resource inspection is unavailable on this OS"), "filesystem inspection is unavailable on this OS"
}
