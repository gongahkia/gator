//go:build !darwin || !cgo

package desktop

import (
	"context"
	"errors"
	"runtime"
)

type unavailableRuntime struct{}

func DefaultRuntime() Runtime { return unavailableRuntime{} }
func unavailableDesktopError() error {
	if runtime.GOOS == "darwin" {
		return errors.New("desktop control requires a cgo-enabled macOS build")
	}
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) AccessibilityTrusted() (bool, error) {
	return false, unavailableDesktopError()
}
func (unavailableRuntime) FrontWindow(context.Context) (Window, error) {
	return Window{}, unavailableDesktopError()
}
func (unavailableRuntime) Screenshot(context.Context, Window) ([]byte, error) {
	return nil, unavailableDesktopError()
}
func (unavailableRuntime) Activate(context.Context, string) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Click(context.Context, float64, float64) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) DoubleClick(context.Context, float64, float64) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Drag(context.Context, []Point) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Move(context.Context, float64, float64) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Scroll(context.Context, float64, float64, int, int) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Type(context.Context, string) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) Press(context.Context, string) error {
	return unavailableDesktopError()
}
func (unavailableRuntime) FocusedFieldSensitive(context.Context, int) (bool, error) {
	return true, unavailableDesktopError()
}
