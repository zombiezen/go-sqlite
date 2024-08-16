// Copyright 2021 Roxy Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"testing"

	"zombiezen.com/go/sqlite"
)

func TestSetAuthorizer(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	authResult := sqlite.AuthResult(0)
	var lastAction sqlite.Action
	auth := sqlite.AuthorizeFunc(func(action sqlite.Action) sqlite.AuthResult {
		lastAction = action
		return authResult
	})
	c.SetAuthorizer(auth)

	t.Run("Allowed", func(t *testing.T) {
		authResult = sqlite.AuthResultOK
		stmt, _, err := c.PrepareTransient("SELECT 1;")
		if err != nil {
			t.Fatal(err)
		}
		stmt.Finalize()
		if lastAction.Type() != sqlite.OpSelect {
			t.Errorf("action = %v; want %v", lastAction, sqlite.OpSelect)
		}
	})

	t.Run("Denied", func(t *testing.T) {
		authResult = sqlite.AuthResultDeny
		stmt, _, err := c.PrepareTransient("SELECT 1;")
		if err == nil {
			stmt.Finalize()
			t.Fatal("PrepareTransient did not return an error")
		}
		if got, want := sqlite.ErrCode(err), sqlite.ResultAuth; got != want {
			t.Errorf("sqlite.ErrCode(err) = %v; want %v", got, want)
		}
		if lastAction.Type() != sqlite.OpSelect {
			t.Errorf("action = %v; want %v", lastAction, sqlite.OpSelect)
		}
	})
}
