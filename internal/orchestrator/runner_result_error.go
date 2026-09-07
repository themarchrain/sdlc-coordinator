package orchestrator

import "errors"

type invalidRunnerResultError struct{ cause error }

func newInvalidRunnerResultError(cause error) error {
	return invalidRunnerResultError{cause: cause}
}

func (e invalidRunnerResultError) Error() string { return e.cause.Error() }

func (e invalidRunnerResultError) Unwrap() error { return e.cause }

type deliveryRunnerResultError struct{ cause error }

func newDeliveryRunnerResultError(cause error) error {
	return deliveryRunnerResultError{cause: cause}
}

func (e deliveryRunnerResultError) Error() string { return e.cause.Error() }

func (e deliveryRunnerResultError) Unwrap() error { return e.cause }

// runnerResultNeedsConvergence reports whether a reconcile error means the
// execution produced a terminal business result that the coordinator cannot
// apply or deliver, so the run must be blocked and the task handed to humans
// instead of being polled again forever.
func runnerResultNeedsConvergence(err error) bool {
	var invalid invalidRunnerResultError
	var delivery deliveryRunnerResultError
	return errors.As(err, &invalid) || errors.As(err, &delivery)
}
