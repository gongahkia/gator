//go:build !darwin

package desktop

import (
	"context"
	"errors"
)

type unavailableRuntime struct{}

func DefaultRuntime() Runtime { return unavailableRuntime{} }
func (unavailableRuntime) AccessibilityTrusted() (bool, error) {
	return false, errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) FrontWindow(context.Context) (Window, error) {
	return Window{}, errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Screenshot(context.Context, Window) ([]byte, error) {
	return nil, errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Activate(context.Context, string) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Click(context.Context, float64, float64) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) DoubleClick(context.Context, float64, float64) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Drag(context.Context, []Point) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Move(context.Context, float64, float64) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Scroll(context.Context, float64, float64, int, int) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Type(context.Context, string) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) Press(context.Context, string) error {
	return errors.New("desktop control is currently available only on macOS")
}
func (unavailableRuntime) FocusedFieldSensitive(context.Context, int) (bool, error) {
	return true, errors.New("desktop control is currently available only on macOS")
}
