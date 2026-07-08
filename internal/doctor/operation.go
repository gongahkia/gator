package doctor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/paw/internal/config"
)

func repairConfigPath(opts Options, sources []configSource, gitRoot string) string {
	if opts.ConfigPath != "" {
		return opts.ConfigPath
	}
	for _, source := range sources {
		if source.Name == "repo" && source.Path != "" && gitRoot != "" {
			return source.Path
		}
	}
	if gitRoot != "" {
		return filepath.Join(gitRoot, ".paw", "config.toml")
	}
	return config.DefaultPath()
}

func backupAndSaveConfig(path string, cfg config.Config) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		backup := path + ".bak"
		if err := copyFile(path, backup, mode); err != nil {
			return err
		}
	}
	return saveConfigWithMode(path, cfg, mode)
}

func saveConfigWithMode(path string, cfg config.Config, mode os.FileMode) error {
	if err := cfg.Save(path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func (r *runner) confirmRepair(action, path string) (bool, string) {
	if r.opts.Yes || r.opts.NonInteractive {
		return true, ""
	}
	if _, err := fmt.Fprintf(r.opts.PromptWriter, "apply repair %s %s? [y/N] ", action, path); err != nil {
		return false, err.Error()
	}
	line, err := bufio.NewReader(r.opts.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err.Error()
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, ""
	default:
		return false, "declined"
	}
}

func (r *runner) logOperation(action, status, path, detail string, opErr error) {
	if r.opts.DryRun || os.Getenv("PAW_DOCTOR_NO_OPLOG") != "" {
		return
	}
	op := Operation{
		Timestamp: r.opts.Now().UTC().Format(time.RFC3339Nano),
		Action:    action,
		Status:    status,
		Path:      path,
		Detail:    detail,
	}
	if opErr != nil {
		op.Error = opErr.Error()
	}
	_ = AppendOperation(r.opts.CWD, op)
}

func OperationLogPath(cwd string) string {
	return filepath.Join(cleanAbs(cwd), ".paw", "doctor", "operations.ndjson")
}

func AppendOperation(cwd string, op Operation) error {
	path := OperationLogPath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return json.NewEncoder(file).Encode(op)
}

func ReadOperations(cwd string, limit int) ([]Operation, error) {
	path := OperationLogPath(cwd)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var ops []Operation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var op Operation
		if err := json.Unmarshal(scanner.Bytes(), &op); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(ops) > limit {
		ops = ops[len(ops)-limit:]
	}
	return ops, nil
}
