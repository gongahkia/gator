package main

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/dependency"
)

// writeDependencyDoctor reports checked-in installation guidance. It does not
// invoke a package manager or fetch an installer: both actions need their own
// explicit provenance, privilege, and upgrade policy.
func writeDependencyDoctor(out io.Writer, statuses []dependency.Status) error {
	if _, err := fmt.Fprintln(out, "Dependencies:"); err != nil {
		return err
	}
	for _, status := range statuses {
		state := "installed"
		usable := status.Installed
		if status.ID == "node" && usable && !supportedNodeTools(status.Path) {
			usable = false
			state = "unsupported version"
		}
		if !status.Installed {
			state = "missing"
		}
		role := "optional"
		if status.Required {
			role = "required"
		}
		if _, err := fmt.Fprintf(out, "  %s: %s (%s for %s)\n", status.Name, state, role, status.Purpose); err != nil {
			return err
		}
		if usable {
			continue
		}
		if status.HelpURL != "" {
			if _, err := fmt.Fprintf(out, "    Official source: %s\n", status.HelpURL); err != nil {
				return err
			}
		}
		if status.Instructions == nil {
			continue
		}
		for _, instruction := range status.Instructions(runtime.GOOS) {
			if _, err := fmt.Fprintf(out, "    %s\n", instruction); err != nil {
				return err
			}
		}
	}
	return nil
}

// External Node is optional tooling; Electron supplies its own application runtime.
func supportedNodeTools(node string) bool {
	version, err := exec.Command(node, "--version").Output()
	if err != nil {
		return false
	}
	major, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(strings.TrimSpace(string(version)), "v"), ".", 2)[0])
	return err == nil && major >= 22
}
