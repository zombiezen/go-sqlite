// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

import "testing"

func TestResultCodeMessage(t *testing.T) {
	t.Log(ResultOK.Message())
	t.Log(ResultNoMem.Message())
}
