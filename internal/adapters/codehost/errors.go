package codehost

import "errors"

type AmbiguousError struct {
	err error
}

func NewAmbiguousError(err error) error {
	if err == nil {
		return nil
	}
	return &AmbiguousError{err: err}
}

func (e *AmbiguousError) Error() string {
	return e.err.Error()
}

func (e *AmbiguousError) Unwrap() error {
	return e.err
}

func IsAmbiguousError(err error) bool {
	var target *AmbiguousError
	return errors.As(err, &target)
}

// TransientRejectionError 表示服务端明确拒绝且确定未应用（HTTP 429/408）。
// 与 AmbiguousError（结果歧义、可能已应用，只能走对账）互斥：这类错误
// 重执行远端调用没有重复写入风险，写回层据此保持可重入。
type TransientRejectionError struct {
	err error
}

func NewTransientRejection(err error) error {
	if err == nil {
		return nil
	}
	return &TransientRejectionError{err: err}
}

func (e *TransientRejectionError) Error() string {
	return e.err.Error()
}

func (e *TransientRejectionError) Unwrap() error {
	return e.err
}

func IsTransientRejection(err error) bool {
	var target *TransientRejectionError
	return errors.As(err, &target)
}
