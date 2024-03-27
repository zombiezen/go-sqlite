// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

import "testing"

func TestResultCodeMessage(t *testing.T) {
	t.Log(ResultOK.Message())
	t.Log(ResultNoMem.Message())
}

func TestErrCode(t *testing.T) {
	rawErr := ResultInterrupt.ToError()
	if got, want := ErrCode(rawErr), ResultInterrupt; got != want {
		t.Errorf("got err=%s, want %s", got, want)
	}

	wrappedErr := errWithMessage{err: rawErr, msg: "Doing something"}
	if got, want := ErrCode(wrappedErr), ResultInterrupt; got != want {
		t.Errorf("got err=%s, want %s", got, want)
	}
}

type errWithMessage struct {
	err error
	msg string
}

func (e errWithMessage) Unwrap() error {
	return e.err
}

func (e errWithMessage) Error() string {
	return e.msg + ": " + e.err.Error()
}
