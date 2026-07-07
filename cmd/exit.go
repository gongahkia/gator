package cmd

import (
	"errors"
	"fmt"
)

type ExitCode int

const (
	ExitSuccess      ExitCode = 0
	ExitSystemError  ExitCode = 1
	ExitUsageError   ExitCode = 2
	ExitVerifyFailed ExitCode = 64
)

type ExitError struct {
	Code ExitCode
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (e *ExitError) ExitCode() ExitCode {
	return e.Code
}

func withExitCode(code ExitCode, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func usageError(err error) error {
	return withExitCode(ExitUsageError, err)
}

func usageErrorf(format string, args ...any) error {
	return usageError(fmt.Errorf(format, args...))
}

func verifyFailedErrorf(format string, args ...any) error {
	return withExitCode(ExitVerifyFailed, fmt.Errorf(format, args...))
}

func codeForError(err error) ExitCode {
	if err == nil {
		return ExitSuccess
	}
	var coded interface{ ExitCode() ExitCode }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return ExitSystemError
}
