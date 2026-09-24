//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices
#include <stdlib.h>
#include <stdint.h>
int gator_desktop_accessibility_trusted(void);
int gator_desktop_front_window(uint32_t *window_id, int *pid, double *x, double *y, double *width, double *height, char *bundle_id, int bundle_size, char *title, int title_size);
int gator_desktop_click(double x, double y);
int gator_desktop_double_click(double x, double y);
int gator_desktop_drag(const double *xs, const double *ys, int count);
int gator_desktop_move(double x, double y);
int gator_desktop_scroll(double x, double y, int delta_x, int delta_y);
int gator_desktop_type(const char *value);
int gator_desktop_press(int keycode);
int gator_desktop_focused_field_sensitive(int pid);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"
)

type darwinRuntime struct{}

func DefaultRuntime() Runtime { return darwinRuntime{} }

func (darwinRuntime) AccessibilityTrusted() (bool, error) {
	return C.gator_desktop_accessibility_trusted() == 1, nil
}

func (darwinRuntime) FrontWindow(ctx context.Context) (Window, error) {
	if err := ctx.Err(); err != nil {
		return Window{}, err
	}
	var windowID C.uint32_t
	var pid C.int
	var x, y, width, height C.double
	bundle := make([]byte, 512)
	title := make([]byte, 2048)
	if C.gator_desktop_front_window(&windowID, &pid, &x, &y, &width, &height, (*C.char)(unsafe.Pointer(&bundle[0])), C.int(len(bundle)), (*C.char)(unsafe.Pointer(&title[0])), C.int(len(title))) != 1 {
		return Window{}, errors.New("could not identify the current macOS application window")
	}
	bundleID := cBuffer(bundle)
	if bundleID == "" {
		return Window{}, errors.New("current macOS application has no bundle identifier")
	}
	return Window{Application: Application{BundleID: bundleID}, Title: cBuffer(title), ID: uint32(windowID), PID: int(pid), X: float64(x), Y: float64(y), Width: float64(width), Height: float64(height)}, nil
}

func (darwinRuntime) Screenshot(ctx context.Context, window Window) ([]byte, error) {
	if window.ID == 0 {
		return nil, errors.New("current application has no capturable window")
	}
	temporary, err := os.CreateTemp("", "gator-desktop-*.png")
	if err != nil {
		return nil, err
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	defer os.Remove(path)
	command := exec.CommandContext(ctx, "/usr/sbin/screencapture", "-x", "-l", strconv.FormatUint(uint64(window.ID), 10), path)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("capture current desktop window: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	png, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, errors.New("macOS did not return a PNG screenshot")
	}
	return png, nil
}

func (darwinRuntime) Activate(ctx context.Context, bundleID string) error {
	if err := validateApplication(Application{BundleID: bundleID}); err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/open", "-b", bundleID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("activate approved desktop application: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (darwinRuntime) Click(ctx context.Context, x, y float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if C.gator_desktop_click(C.double(x), C.double(y)) != 1 {
		return errors.New("macOS rejected the desktop click; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) DoubleClick(ctx context.Context, x, y float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if C.gator_desktop_double_click(C.double(x), C.double(y)) != 1 {
		return errors.New("macOS rejected the desktop double click; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) Drag(ctx context.Context, points []Point) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(points) < 2 {
		return errors.New("desktop drag needs at least two points")
	}
	xs, ys := make([]C.double, len(points)), make([]C.double, len(points))
	for index, point := range points {
		xs[index], ys[index] = C.double(point.X), C.double(point.Y)
	}
	if C.gator_desktop_drag(&xs[0], &ys[0], C.int(len(points))) != 1 {
		return errors.New("macOS rejected the desktop drag; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) Move(ctx context.Context, x, y float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if C.gator_desktop_move(C.double(x), C.double(y)) != 1 {
		return errors.New("macOS rejected the desktop pointer move; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) Scroll(ctx context.Context, x, y float64, deltaX, deltaY int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if C.gator_desktop_scroll(C.double(x), C.double(y), C.int(deltaX), C.int(deltaY)) != 1 {
		return errors.New("macOS rejected the desktop scroll; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) Type(ctx context.Context, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text := C.CString(value)
	defer C.free(unsafe.Pointer(text))
	if C.gator_desktop_type(text) != 1 {
		return errors.New("macOS rejected desktop typing; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) Press(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	code, ok := keyCode(key)
	if !ok {
		return errors.New("desktop key is unavailable")
	}
	if C.gator_desktop_press(C.int(code)) != 1 {
		return errors.New("macOS rejected the desktop key press; grant Accessibility permission to Gator")
	}
	return nil
}

func (darwinRuntime) FocusedFieldSensitive(ctx context.Context, pid int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return true, err
	}
	value := C.gator_desktop_focused_field_sensitive(C.int(pid))
	if value < 0 {
		return true, errors.New("could not inspect the focused desktop field; Gator blocks typing when field safety is unknown")
	}
	return value == 1, nil
}

func cBuffer(value []byte) string {
	if position := strings.IndexByte(string(value), 0); position >= 0 {
		return string(value[:position])
	}
	return string(value)
}

func keyCode(key string) (int, bool) {
	codes := map[string]int{"Enter": 36, "Tab": 48, "Space": 49, "Backspace": 51, "Escape": 53, "Delete": 117, "Left": 123, "Right": 124, "Down": 125, "Up": 126, "Home": 115, "End": 119, "PageUp": 116, "PageDown": 121}
	value, ok := codes[key]
	return value, ok
}
