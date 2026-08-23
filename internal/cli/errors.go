package cli

import "errors"

// ExitCodeError is an error that carries an explicit process exit code,
// distinguishing a deliberate stopped-run (1) from a load/parse/config
// error (2) — see docs/plans/go-rewrite.md §11's exit-code decision.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string { return e.Err.Error() }
func (e *ExitCodeError) Unwrap() error { return e.Err }

// NewLoadError wraps err as a load/parse/config failure (exit code 2).
func NewLoadError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitCodeError{Code: 2, Err: err}
}

// NewRunError wraps err as a stopped-run failure (exit code 1).
func NewRunError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitCodeError{Code: 1, Err: err}
}

// ExitCodeFor returns the process exit code for err, defaulting to 1 for
// any error not explicitly classified.
func ExitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	return 1
}

