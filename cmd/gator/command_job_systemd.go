package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const jobSystemdUnitName = "gator-work-supervisor.service"

// jobSystemd manages only Gator's per-user unit. It does not enable lingering,
// so jobs run while the Linux user manager is active, not while the host is off.
func jobSystemd(arguments []string, out io.Writer) error {
	if runtime.GOOS != "linux" {
		return errors.New("job systemd scheduling is available only on Linux")
	}
	if len(arguments) == 0 {
		return errors.New("usage: gator job systemd install|status|remove --yes")
	}
	_, stateDir, err := jobStore()
	if err != nil {
		return err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	unitPath := filepath.Join(configDir, "systemd", "user", jobSystemdUnitName)
	command := func(args ...string) ([]byte, error) {
		return exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	}
	switch arguments[0] {
	case "install":
		if len(arguments) != 1 {
			return errors.New("usage: gator job systemd install")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if !filepath.IsAbs(executable) {
			return errors.New("Gator executable path must be absolute for systemd installation")
		}
		unit, err := systemdJobUnit(executable, stateDir, os.Getenv("GATOR_CONFIG_DIR"))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o700); err != nil {
			return err
		}
		if err := writePrivateFile(unitPath, []byte(unit)); err != nil {
			return err
		}
		if output, err := command("daemon-reload"); err != nil {
			return fmt.Errorf("reload user systemd units: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		if output, err := command("enable", "--now", jobSystemdUnitName); err != nil {
			return fmt.Errorf("enable Gator user service: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		// enable --now does not restart a unit that was already active. Restart
		// after daemon-reload so reinstalling a moved binary takes effect.
		if output, err := command("restart", jobSystemdUnitName); err != nil {
			return fmt.Errorf("restart Gator user service: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		_, err = fmt.Fprintf(out, "Installed local user service %s\n  unit: %s\nIt runs while your Linux user manager is active. Desktop/CUA sessions are never scheduled.\n", jobSystemdUnitName, unitPath)
		return err
	case "status":
		if len(arguments) != 1 {
			return errors.New("usage: gator job systemd status")
		}
		output, err := command("--no-pager", "status", jobSystemdUnitName)
		if err != nil {
			return fmt.Errorf("Gator user service is not running: %s", strings.TrimSpace(string(output)))
		}
		_, err = out.Write(output)
		return err
	case "remove":
		if len(arguments) != 2 || arguments[1] != "--yes" {
			return errors.New("usage: gator job systemd remove --yes")
		}
		if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Gator user service unit is not installed at %s", unitPath)
		} else if err != nil {
			return err
		}
		if output, err := command("disable", "--now", jobSystemdUnitName); err != nil {
			return fmt.Errorf("disable Gator user service: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		if err := os.Remove(unitPath); err != nil {
			return err
		}
		if output, err := command("daemon-reload"); err != nil {
			return fmt.Errorf("reload user systemd units: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		_, err = fmt.Fprintf(out, "Removed local user service %s.\n", jobSystemdUnitName)
		return err
	default:
		return errors.New("usage: gator job systemd install|status|remove --yes")
	}
}

func systemdJobUnit(executable, stateDir, configDir string) (string, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(stateDir) {
		return "", errors.New("systemd job paths must be absolute")
	}
	execValue, err := quoteSystemdValue(executable)
	if err != nil {
		return "", err
	}
	stateValue, err := quoteSystemdValue("GATOR_STATE_DIR=" + stateDir)
	if err != nil {
		return "", err
	}
	var environment string
	if strings.TrimSpace(configDir) != "" {
		if !filepath.IsAbs(configDir) {
			return "", errors.New("GATOR_CONFIG_DIR must be absolute for systemd installation")
		}
		configValue, err := quoteSystemdValue("GATOR_CONFIG_DIR=" + configDir)
		if err != nil {
			return "", err
		}
		environment = "Environment=" + configValue + "\n"
	}
	return "[Unit]\nDescription=Gator local job supervisor\n\n[Service]\nType=simple\nExecStart=" + execValue + " job supervisor\nEnvironment=" + stateValue + "\n" + environment + "Restart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n", nil
}

func quoteSystemdValue(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("systemd unit value contains a control character")
	}
	// systemd expands percent specifiers even inside quoted values.
	return strconv.Quote(strings.ReplaceAll(value, "%", "%%")), nil
}
