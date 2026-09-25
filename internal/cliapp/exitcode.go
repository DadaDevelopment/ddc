package cliapp

import (
	"errors"
	"fmt"
)

const (
	ExitPass   = 0
	ExitFail   = 1
	ExitConfig = 2
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

func (e *ExitError) Unwrap() error { return e.Err }

func ConfigError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: ExitConfig, Err: err}
}

func ConfigErrorf(format string, args ...any) error {
	return ConfigError(fmt.Errorf(format, args...))
}

func ExitCode(err error) int {
	if err == nil {
		return ExitPass
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return ExitFail
}

func childExitCode(code int) int {
	switch code {
	case ExitPass, ExitConfig:
		return code
	}
	return ExitFail
}

func worstExitCode(codes ...int) int {
	worst := ExitPass
	for _, c := range codes {
		if c == ExitConfig {
			return ExitConfig
		}
		if c > worst {
			worst = c
		}
	}
	return worst
}
