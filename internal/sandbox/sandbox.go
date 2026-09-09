// Package sandbox prepares host processes for constrained Gator execution.
//
// A Git worktree is an edit-isolation mechanism, not a process sandbox. This
// package provides the separate operating-system boundary used for commands
// that the agent asks Gator to run.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Mode controls whether a command must use an operating-system sandbox.
type Mode string

const (
	// Strict refuses to start a command unless Gator can establish a platform
	// sandbox. It is the product default.
	Strict Mode = "strict"
	// Off intentionally runs a command with the Gator user's host permissions.
	// It exists for a developer who explicitly accepts that tradeoff.
	Off Mode = "off"
)

// Network controls outbound network access from the sandboxed command.
type Network string

const (
	DenyNetwork  Network = "deny"
	AllowNetwork Network = "allow"
)

// Policy is the explicit host-execution boundary for one Gator run. Additional
// filesystem roots and environment variables are opt-in because both are
// capability grants to code produced by an agent.
type Policy struct {
	// WritablePaths narrows writes within the worktree; empty retains full private-root access.
	WritablePaths []string `json:"writable_paths,omitempty"`
	Mode          Mode     `json:"mode,omitempty"`
	Network       Network  `json:"network,omitempty"`
	Environment   []string `json:"environment,omitempty"`
	ReadOnlyRoots []string `json:"read_only_roots,omitempty"`
	WritableRoots []string `json:"writable_roots,omitempty"`
}

const maxRoots = 64

var environmentName = regexp.MustCompile(`\A[A-Za-z_][A-Za-z0-9_]{0,127}\z`)

// DefaultPolicy is secure by default: commands may read and write their
// dedicated worktree, but have no network and no access to the developer's
// other files unless a policy grants it.
func DefaultPolicy() Policy {
	return Policy{Mode: Strict, Network: DenyNetwork}
}

// Normalize resolves omitted values and returns an independent copy suitable
// for process execution.
func (p Policy) Normalize() Policy {
	if p.Mode == "" {
		p.Mode = Strict
	}
	if p.Network == "" {
		p.Network = DenyNetwork
	}
	p.WritablePaths = append([]string(nil), p.WritablePaths...)
	p.Environment = append([]string(nil), p.Environment...)
	p.ReadOnlyRoots = append([]string(nil), p.ReadOnlyRoots...)
	p.WritableRoots = append([]string(nil), p.WritableRoots...)
	return p
}

// Validate rejects malformed capability grants before a command starts.
func (p Policy) Validate() error {
	p = p.Normalize()
	if p.Mode != Strict && p.Mode != Off {
		return fmt.Errorf("unknown sandbox mode %q", p.Mode)
	}
	if p.Network != DenyNetwork && p.Network != AllowNetwork {
		return fmt.Errorf("unknown sandbox network mode %q", p.Network)
	}
	for _, path := range p.WritablePaths {
		if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return errors.New("invalid worktree writable path")
		}
	}
	if len(p.WritablePaths) > 0 && p.Mode != Strict {
		return errors.New("a bounded write envelope requires strict sandboxing")
	}
	if len(p.Environment) > 128 {
		return errors.New("sandbox policy has too many environment variables")
	}
	seenEnvironment := make(map[string]struct{}, len(p.Environment))
	for _, name := range p.Environment {
		if !environmentName.MatchString(name) {
			return fmt.Errorf("invalid sandbox environment variable %q", name)
		}
		if _, exists := seenEnvironment[name]; exists {
			return fmt.Errorf("sandbox environment variable %q is listed more than once", name)
		}
		seenEnvironment[name] = struct{}{}
	}
	if err := validateRoots("read-only", p.ReadOnlyRoots); err != nil {
		return err
	}
	return validateRoots("writable", p.WritableRoots)
}

func validateRoots(label string, roots []string) error {
	if len(roots) > maxRoots {
		return fmt.Errorf("sandbox policy has too many %s roots", label)
	}
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
			return fmt.Errorf("sandbox %s root must be an absolute path", label)
		}
		clean := filepath.Clean(root)
		if clean == string(filepath.Separator) {
			return fmt.Errorf("sandbox %s root must not be the filesystem root", label)
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("sandbox %s root %q is listed more than once", label, root)
		}
		seen[clean] = struct{}{}
	}
	return nil
}

// Request describes one process to prepare. Dir must be the canonical Gator
// worktree path; it is always added as a writable root.
type Request struct {
	Dir  string
	Argv []string
	// Policy must be validated by the caller or will be validated by Prepare.
	Policy Policy
}

// Prepared is an executable command plus cleanup for its private scratch
// directory. Cleanup is idempotent and must be called after Command.Run.
type Prepared struct {
	Command *exec.Cmd
	Cleanup func()
}

// Prepare creates a constrained process command. Strict mode has native
// implementations on macOS and Linux. Other platforms fail closed until a
// comparably strong implementation is available.
func Prepare(ctx context.Context, request Request) (Prepared, error) {
	if len(request.Argv) == 0 || strings.TrimSpace(request.Argv[0]) == "" {
		return Prepared{}, errors.New("sandbox command argv is required")
	}
	if strings.TrimSpace(request.Dir) == "" {
		return Prepared{}, errors.New("sandbox worktree directory is required")
	}
	root, err := filepath.EvalSymlinks(request.Dir)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve sandbox worktree: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Prepared{}, fmt.Errorf("stat sandbox worktree: %w", err)
	}
	if !info.IsDir() {
		return Prepared{}, errors.New("sandbox worktree must be a directory")
	}
	policy := request.Policy.Normalize()
	if err := policy.Validate(); err != nil {
		return Prepared{}, err
	}
	environment, scratch, cleanup, err := processEnvironment(policy)
	if err != nil {
		return Prepared{}, err
	}
	if policy.Mode == Off {
		command := exec.CommandContext(ctx, request.Argv[0], request.Argv[1:]...)
		command.Dir = root
		command.Env = environment
		return Prepared{Command: command, Cleanup: cleanup}, nil
	}

	switch runtime.GOOS {
	case "darwin":
		command, err := prepareDarwin(ctx, root, scratch, request.Argv, environment, policy)
		if err != nil {
			cleanup()
			return Prepared{}, err
		}
		return Prepared{Command: command, Cleanup: cleanup}, nil
	case "linux":
		command, err := prepareLinux(ctx, root, scratch, request.Argv, environment, policy)
		if err != nil {
			cleanup()
			return Prepared{}, err
		}
		return Prepared{Command: command, Cleanup: cleanup}, nil
	default:
		cleanup()
		return Prepared{}, fmt.Errorf("strict sandbox is unavailable on %s; use sandbox mode off only when you intentionally accept host access", runtime.GOOS)
	}
}

// StrictAvailability reports the platform mechanism a strict run will use. It
// deliberately verifies only the executable prerequisite; namespace and
// policy failures are still surfaced when the command is prepared.
func StrictAvailability() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err != nil {
			return "", errors.New("sandbox-exec is unavailable")
		}
		return "macOS Seatbelt (sandbox-exec)", nil
	case "linux":
		if _, err := exec.LookPath("bwrap"); err != nil {
			return "", errors.New("bubblewrap (bwrap) is unavailable")
		}
		return "Bubblewrap", nil
	default:
		return "", fmt.Errorf("strict sandbox is unavailable on %s", runtime.GOOS)
	}
}

func processEnvironment(policy Policy) ([]string, string, func(), error) {
	scratch, err := os.MkdirTemp("", "gator-sandbox-")
	if err != nil {
		return nil, "", nil, fmt.Errorf("create sandbox scratch directory: %w", err)
	}
	canonicalScratch, err := filepath.EvalSymlinks(scratch)
	if err != nil {
		_ = os.RemoveAll(scratch)
		return nil, "", nil, fmt.Errorf("resolve sandbox scratch directory: %w", err)
	}
	scratch = canonicalScratch
	cleanup := func() { _ = os.RemoveAll(scratch) }
	values := map[string]string{
		"PATH":    safeEnvironment("PATH"),
		"TMPDIR":  scratch,
		"TMP":     scratch,
		"TEMP":    scratch,
		"HOME":    scratch,
		"GOCACHE": filepath.Join(scratch, "go-build"),
		"LANG":    safeEnvironment("LANG"),
		"LC_ALL":  safeEnvironment("LC_ALL"),
		"TERM":    safeEnvironment("TERM"),
	}
	if moduleCache := goModuleCache(); moduleCache != "" {
		values["GOMODCACHE"] = moduleCache
	}
	for _, name := range policy.Environment {
		values[name] = os.Getenv(name)
	}
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if value != "" {
			keys = append(keys, key+"="+value)
		}
	}
	sort.Strings(keys)
	return keys, scratch, cleanup, nil
}

func safeEnvironment(name string) string {
	return os.Getenv(name)
}

// goModuleCache is source and module metadata rather than a general-purpose
// user directory. Exposing it read-only keeps ordinary Go verification usable
// with the default network-denied policy, while GOCACHE remains per-command.
func goModuleCache() string {
	cache := os.Getenv("GOMODCACHE")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cache = filepath.Join(home, "go", "pkg", "mod")
	}
	canonical, err := filepath.EvalSymlinks(cache)
	if err != nil {
		return ""
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return ""
	}
	return canonical
}

// Keep filesystem seams local to this package for platform-independent tests.
var (
	osReadFile = os.ReadFile
	osStat     = os.Stat
)
