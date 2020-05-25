package sqlite_test

import (
	"testing"

	"crawshaw.io/sqlite"
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
	var lastAction sqlite.OpType
	auth := sqlite.AuthorizeFunc(func(info sqlite.ActionInfo) sqlite.AuthResult {
		lastAction = info.Action
		return authResult
	})
	c.SetAuthorizer(auth)

	t.Run("Allowed", func(t *testing.T) {
		authResult = 0
		stmt, _, err := c.PrepareTransient("SELECT 1;")
		if err != nil {
			t.Fatal(err)
		}
		stmt.Finalize()
		if lastAction != sqlite.SQLITE_SELECT {
			t.Errorf("action = %q; want SQLITE_SELECT", lastAction)
		}
	})

	t.Run("Denied", func(t *testing.T) {
		authResult = sqlite.SQLITE_DENY
		stmt, _, err := c.PrepareTransient("SELECT 1;")
		if err == nil {
			stmt.Finalize()
			t.Fatal("PrepareTransient did not return an error")
		}
		if got, want := sqlite.ErrCode(err), sqlite.SQLITE_AUTH; got != want {
			t.Errorf("sqlite.ErrCode(err) = %v; want %v", got, want)
		}
		if lastAction != sqlite.SQLITE_SELECT {
			t.Errorf("action = %q; want SQLITE_SELECT", lastAction)
		}
	})
}
