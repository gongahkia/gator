//go:build !darwin && !linux

package tmux

type OSProbe struct{}

func (OSProbe) Process(int) (string, error)   { return "", ErrUnavailable }
func (OSProbe) Socket(string) (string, error) { return "", ErrUnavailable }
