package repository

import "errors"

var (
	ErrNotFound                   = errors.New("not found")
	ErrTaskNotClaimable           = errors.New("task not claimable")
	ErrRecoveryGenerationMismatch = errors.New("recovery generation mismatch")
	ErrTaskLeaseNotRenewable      = errors.New("task lease not renewable")
	ErrResultAlreadyAccepted      = errors.New("result already accepted")
	ErrResultNotAcceptable        = errors.New("result not acceptable")
	ErrResultNotClaimable         = errors.New("result not claimable")
	ErrStopNotCurrentRun          = errors.New("not_current_run")
	ErrStopAlreadyTerminal        = errors.New("already_terminal")
	ErrInvalidStopRequest         = errors.New("invalid stop request")
	ErrTaskInterrupted            = errors.New("task interrupted")
)
