package repository

import "errors"

var (
	ErrNotFound              = errors.New("not found")
	ErrTaskNotClaimable      = errors.New("task not claimable")
	ErrTaskLeaseNotRenewable = errors.New("task lease not renewable")
	ErrResultAlreadyAccepted = errors.New("result already accepted")
	ErrResultNotAcceptable   = errors.New("result not acceptable")
	ErrResultNotClaimable    = errors.New("result not claimable")
)
