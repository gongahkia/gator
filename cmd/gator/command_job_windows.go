package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/privatefs"
	"golang.org/x/sys/windows/registry"
)

const windowsRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const windowsRunName = "GatorBackgroundChecks"
const windowsRunPath = `HKEY_CURRENT_USER\` + windowsRunKey + `\` + windowsRunName

type windowsLoginProfile struct {
	Version    int    `json:"version"`
	Executable string `json:"executable"`
	StateDir   string `json:"state_dir"`
	ConfigDir  string `json:"config_dir"`
}

func windowsLoginProfilePath() (string, error) {
	base, err := os.UserCacheDir()
	return filepath.Join(base, "Gator", "background", "profile.json"), err
}

func windowsLoginCommand(binary string) string {
	// Always quote the executable, even if it currently has no spaces.
	return `"` + binary + `" job supervisor --login-profile`
}

func readWindowsLoginCommand() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, kind, err := key.GetStringValue(windowsRunName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if kind != registry.SZ {
		return "", errors.New("Gator start-at-login entry is not an ordinary command")
	}
	return value, nil
}

func readWindowsLoginProfile() (windowsLoginProfile, []byte, error) {
	var profile windowsLoginProfile
	path, err := windowsLoginProfilePath()
	if err != nil {
		return profile, nil, err
	}
	if err := privatefs.Check(path); err != nil {
		return profile, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return profile, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil {
		return profile, nil, err
	}
	if len(data) > 64*1024 {
		return profile, data, errors.New("start-at-login profile is too large")
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, data, err
	}
	if profile.Version != 1 || !filepath.IsAbs(profile.Executable) || !filepath.IsAbs(profile.StateDir) || !filepath.IsAbs(profile.ConfigDir) || strings.ContainsAny(profile.Executable+profile.StateDir+profile.ConfigDir, "\x00\r\n\"") {
		return profile, data, errors.New("invalid start-at-login profile")
	}
	return profile, data, nil
}

func loadLoginProfile() error {
	profile, _, err := readWindowsLoginProfile()
	if err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(binary), filepath.Clean(profile.Executable)) {
		return errors.New("start-at-login executable changed; enable it again from Recovery")
	}
	if err := os.Setenv("GATOR_STATE_DIR", profile.StateDir); err != nil {
		return err
	}
	return os.Setenv("GATOR_CONFIG_DIR", profile.ConfigDir)
}

func reviewWindowsLogin(stateDir string) (installed, owns bool, digest string, err error) {
	command, err := readWindowsLoginCommand()
	if err != nil {
		return false, false, "", err
	}
	if command == "" {
		return false, false, "absent", nil
	}
	profile, data, profileErr := readWindowsLoginProfile()
	sum := sha256.Sum256(append(append([]byte(command), 0), data...))
	digest = hex.EncodeToString(sum[:])
	if profileErr != nil {
		return true, false, digest, nil
	}
	return true, command == windowsLoginCommand(profile.Executable) && sameWindowsProfilePath(profile.StateDir, stateDir), digest, nil
}

func sameWindowsProfilePath(a, b string) bool {
	if left, err := os.Stat(a); err == nil {
		if right, err := os.Stat(b); err == nil {
			return os.SameFile(left, right)
		}
	}
	canonical := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Clean(resolved)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		return filepath.Clean(absolute)
	}
	return canonical(a) == canonical(b)
}

// Register only a per-user Run value. The small command reads private profile
// settings, so long state/config paths do not exceed Run's 260-character limit.
// The runtime uses the Windows GUI subsystem, so login opens no console.
func jobWindows(arguments []string, out io.Writer) error {
	if len(arguments) < 1 {
		return errors.New("missing Windows login action")
	}
	_, stateDir, err := jobStore()
	if err != nil {
		return err
	}
	installed, owns, digest, err := reviewWindowsLogin(stateDir)
	if err != nil {
		return err
	}
	if arguments[0] == "status" {
		_, err := fmt.Fprintf(out, "Windows start at login: installed=%t, selected profile=%t\n", installed, owns)
		return err
	}
	if len(arguments) != 3 || arguments[1] != "--expected" || arguments[2] != digest {
		return errors.New("start-at-login changed during review; refresh Recovery")
	}
	if installed && !owns {
		return errors.New("start-at-login belongs to another or unrecognized profile")
	}
	if arguments[0] == "remove" {
		key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer key.Close()
		if err := key.DeleteValue(windowsRunName); err != nil {
			return err
		}
		_ = jobSupervisorRequest("stop", io.Discard)
		// Registration is removed even if the supervisor was already offline.
		_, err = fmt.Fprintln(out, "Disabled Windows start at login. Saved checks remain.")
		return err
	}
	if arguments[0] != "install" {
		return errors.New("unknown Windows login action")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	configuration, err := config.ResolveDir("")
	if err != nil {
		return err
	}
	profile := windowsLoginProfile{Version: 1, Executable: binary, StateDir: stateDir, ConfigDir: configuration}
	command := windowsLoginCommand(binary)
	if len(utf16.Encode([]rune(command))) > 260 || strings.ContainsAny(binary, "\x00\r\n\"") {
		return errors.New("application path is too long for Windows start at login; install Gator in a shorter location")
	}
	path, err := windowsLoginProfilePath()
	if err != nil {
		return err
	}
	if err := privatefs.MkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	if err := writePrivateFile(path, data); err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue(windowsRunName, command); err != nil {
		return err
	}
	if err := startLocalSupervisor(stateDir); err != nil {
		return fmt.Errorf("start at login is configured; could not start current checks: %w", err)
	}
	_, err = fmt.Fprintln(out, "Enabled Windows start at login for this profile. Checks run while this computer is on and you are signed in.")
	return err
}
