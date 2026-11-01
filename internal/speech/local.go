package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/hostproc"
)

const LocalTimeout = 180 * time.Second

type LocalConfig struct {
	Store      *LocalStore
	Executable string
}

// ResolveLocalRuntime only inspects the trusted host installation; it neither
// starts a daemon nor downloads/executes anything during ordinary app startup.
func ResolveLocalRuntime() (string, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", errors.New("local Whisper currently supports macOS and Linux; Windows remains a preview")
	}
	executable := strings.TrimSpace(os.Getenv("GATOR_WHISPER_BINARY"))
	if executable == "" {
		executable = "whisper-cli"
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		return "", errors.New("install whisper.cpp (whisper-cli) once to use local dictation; model downloads are available below")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("the local Whisper runtime is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("the local Whisper runtime must be an ordinary executable")
	}
	return path, nil
}

// ValidateLocalWAV accepts only bounded mono 16 kHz signed 16-bit PCM. Browser
// recordings are converted before IPC; no external converter or shell is used.
func ValidateLocalWAV(data []byte) error {
	if len(data) < 44 || len(data) > MaxAudioBytes || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return errors.New("local dictation requires a valid PCM WAV recording")
	}
	format, frames := false, false
	for offset := 12; offset < len(data); {
		if offset+8 > len(data) {
			return errors.New("invalid WAV chunk")
		}
		length := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := uint64(offset) + 8 + length
		if end > uint64(len(data)) {
			return errors.New("invalid WAV chunk length")
		}
		chunk := data[offset+8 : int(end)]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if format || len(chunk) < 16 || binary.LittleEndian.Uint16(chunk[:2]) != 1 || binary.LittleEndian.Uint16(chunk[2:4]) != 1 || binary.LittleEndian.Uint32(chunk[4:8]) != 16000 || binary.LittleEndian.Uint32(chunk[8:12]) != 32000 || binary.LittleEndian.Uint16(chunk[12:14]) != 2 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return errors.New("local dictation requires mono 16 kHz, 16-bit PCM")
			}
			format = true
		case "data":
			if frames || length == 0 || length%2 != 0 || length > 120*32000 {
				return errors.New("record between a moment and two minutes of local audio")
			}
			frames = true
		}
		offset = int(end + length%2)
		if offset > len(data) {
			return errors.New("invalid WAV padding")
		}
	}
	if !format || !frames {
		return errors.New("WAV format or audio data is missing")
	}
	return nil
}

func transcribeLocal(ctx context.Context, config Config, recording Recording) (Result, error) {
	result := Result{Provider: LocalProvider, Model: config.Model, Reported: true}
	if result.Model == "" {
		result.Model = DefaultLocalModel
	}
	if config.Local == nil || config.Local.Store == nil || config.Local.Executable == "" {
		return result, errors.New("set up local Whisper in Voice settings first")
	}
	if mime, _, _ := strings.Cut(recording.MediaType, ";"); mime != "audio/wav" {
		return result, errors.New("convert local recordings to PCM WAV before transcription")
	}
	if err := ValidateLocalWAV(recording.Data); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, LocalTimeout)
	defer cancel()
	store := config.Local.Store
	root, err := os.OpenRoot(store.directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	lease, err := filelock.OpenRootFile(root, ".inference.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	if err := filelock.Lock(lease, filelock.Exclusive|filelock.Nonblocking); err != nil {
		return result, errors.New("another local transcription is running; try again when it finishes")
	}
	defer filelock.Unlock(lease)
	// Serialize model replacement with inference across short-lived helpers.
	download, err := filelock.OpenRootFile(root, ".download.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return result, err
	}
	defer download.Close()
	if err := filelock.Lock(download, filelock.Shared|filelock.Nonblocking); err != nil {
		return result, errors.New("wait for the voice model download to finish")
	}
	defer filelock.Unlock(download)
	model, err := store.VerifiedModel(ctx, result.Model)
	if err != nil {
		return result, err
	}
	working, err := os.MkdirTemp(store.directory, ".runtime-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(working)
	language := "auto"
	if recording.Language != "" {
		language = strings.ToLower(strings.Split(recording.Language, "-")[0])
	}
	if language != "auto" && (len(language) < 2 || len(language) > 3 || strings.IndexFunc(language, func(r rune) bool { return r < 'a' || r > 'z' }) >= 0) {
		return result, errors.New("unsupported local dictation language")
	}
	// An explicit non-'-' output basename preserves the live stdout callback
	// with stdin input in newer whisper.cpp. No export-format flags are enabled:
	// neither this basename nor the recording is written as an output file.
	command := hostproc.CommandContext(ctx, config.Local.Executable, "--model", model, "--file", "-", "--output-file", "gator-stdin", "--no-timestamps", "--no-prints", "--no-gpu", "--language", language, "--threads", "4")
	command.Dir = working
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + working, "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	command.Stdin = bytes.NewReader(recording.Data)
	command.WaitDelay = 2 * time.Second
	stdout := &localOutput{limit: MaxTranscriptBytes, cancel: cancel}
	stderr := &localOutput{limit: 64 * 1024, cancel: cancel, discard: true}
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if stdout.exceeded || stderr.exceeded {
		return result, errors.New("local speech process exceeded its output limit")
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("local dictation stopped: %w", ctx.Err())
	}
	if err != nil {
		return result, errors.New("local Whisper could not transcribe this recording; check its installation and selected model")
	}
	result.Text = strings.TrimSpace(stdout.buffer.String())
	if !utf8.ValidString(result.Text) || strings.ContainsAny(result.Text, "\x00\x1b") {
		return result, errors.New("local Whisper returned invalid transcript text")
	}
	if result.Text == "" {
		return result, errors.New("no speech was recognized; try a clearer recording")
	}
	return result, nil
}

type localOutput struct {
	mu                sync.Mutex
	buffer            bytes.Buffer
	size, limit       int
	exceeded, discard bool
	cancel            context.CancelFunc
}

func (w *localOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.size += len(p)
	if w.size > w.limit {
		w.exceeded = true
		w.cancel()
		return 0, io.ErrShortBuffer
	}
	if !w.discard {
		_, _ = w.buffer.Write(p)
	}
	return len(p), nil
}
