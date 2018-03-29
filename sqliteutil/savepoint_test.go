// Copyright (c) 2018 David Crawshaw <david@zentus.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package sqliteutil

import (
	"errors"
	"sqlite"
	"strings"
	"testing"
)

func TestExec(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := Exec(conn, "CREATE TABLE t (c1);", nil); err != nil {
		t.Fatal(err)
	}
	countFn := func() int {
		var count int
		fn := func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		}
		if err := Exec(conn, "SELECT count(*) FROM t;", fn); err != nil {
			t.Fatal(err)
		}
		return count
	}
	errNoSuccess := errors.New("succeed=false")
	insert := func(succeed bool) (err error) {
		defer Save(conn)(&err)

		if err := Exec(conn, `INSERT INTO t VALUES ("hello");`, nil); err != nil {
			t.Fatal(err)
		}

		if succeed {
			return nil
		}
		return errNoSuccess
	}

	if err := insert(true); err != nil {
		t.Fatal(err)
	}
	if got := countFn(); got != 1 {
		t.Errorf("expecting 1 row, got %d", got)
	}
	if err := insert(true); err != nil {
		t.Fatal(err)
	}
	if got := countFn(); got != 2 {
		t.Errorf("expecting 2 rows, got %d", got)
	}
	if err := insert(false); err != errNoSuccess {
		t.Errorf("expecting insert to fail with errNoSuccess, got %v", err)
	}
	if got := countFn(); got != 2 {
		t.Errorf("expecting 2 rows, got %d", got)
	}
}

func TestPanic(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := Exec(conn, "CREATE TABLE t (c1);", nil); err != nil {
		t.Fatal(err)
	}
	if err := Exec(conn, `INSERT INTO t VALUES ("one");`, nil); err != nil {
		t.Fatal(err)
	}

	defer func() {
		p := recover()
		if p == nil {
			t.Errorf("panic expected")
		}
		if err, isErr := p.(error); !isErr || !strings.Contains(err.Error(), "sqlite") {
			t.Errorf("panic is not an sqlite error: %v", err)
		}

		count := 0
		fn := func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		}
		if err := Exec(conn, "SELECT count(*) FROM t;", fn); err != nil {
			t.Error(err)
		}
		if count != 1 {
			t.Errorf("got %d rows, want 1", count)
		}
	}()

	if err := doPanic(conn); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func doPanic(conn *sqlite.Conn) (err error) {
	defer Save(conn)(&err)

	if err := Exec(conn, `INSERT INTO t VALUES ("hello");`, nil); err != nil {
		return err
	}

	conn.Prep("SELECT bad query") // panics
	return nil
}

func TestDone(t *testing.T) {
	doneCh := make(chan struct{})

	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.SetInterrupt(doneCh)
	close(doneCh)

	relFn := Save(conn)
	relFn(&err)
	if code := sqlite.ErrCode(err); code != sqlite.SQLITE_INTERRUPT {
		t.Errorf("savepoint release function error code is %v, want SQLITE_INTERRUPT", code)
	}
}
