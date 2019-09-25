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

package sqlitex_test

import (
	"fmt"
	"reflect"
	"testing"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

func TestExec(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := sqlitex.ExecTransient(conn, "CREATE TABLE t (a TEXT, b INTEGER);", nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Exec(conn, "INSERT INTO t (a, b) VALUES (?, ?);", nil, "a1", 1); err != nil {
		t.Error(err)
	}
	if err := sqlitex.Exec(conn, "INSERT INTO t (a, b) VALUES (?, ?);", nil, "a2", 2); err != nil {
		t.Error(err)
	}

	var a []string
	var b []int64
	fn := func(stmt *sqlite.Stmt) error {
		a = append(a, stmt.ColumnText(0))
		b = append(b, stmt.ColumnInt64(1))
		return nil
	}
	if err := sqlitex.ExecTransient(conn, "SELECT a, b FROM t;", fn); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a1", "a2"}; !reflect.DeepEqual(a, want) {
		t.Errorf("a=%v, want %v", a, want)
	}
	if want := []int64{1, 2}; !reflect.DeepEqual(b, want) {
		t.Errorf("b=%v, want %v", b, want)
	}
}

func TestExecErr(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	err = sqlitex.Exec(conn, "INVALID SQL STMT", nil)
	if err == nil {
		t.Error("invalid SQL did not return an error code")
	}
	if got, want := sqlite.ErrCode(err), sqlite.SQLITE_ERROR; got != want {
		t.Errorf("INVALID err code=%s, want %s", got, want)
	}

	if err := sqlitex.Exec(conn, "CREATE TABLE t (c1, c2);", nil); err != nil {
		t.Error(err)
	}
	if err := sqlitex.Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 1, 1); err != nil {
		t.Error(err)
	}
	if err := sqlitex.Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 2, 2); err != nil {
		t.Error(err)
	}
	err = sqlitex.Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 1, 1, 1)
	if got, want := sqlite.ErrCode(err), sqlite.SQLITE_RANGE; got != want {
		t.Errorf("INSERT err code=%s, want %s", got, want)
	}

	calls := 0
	customErr := fmt.Errorf("custom err")
	fn := func(stmt *sqlite.Stmt) error {
		calls++
		return customErr
	}
	err = sqlitex.Exec(conn, "SELECT c1 FROM t;", fn)
	if err != customErr {
		t.Errorf("SELECT want err=customErr, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("SELECT want truncated callback calls, got calls=%d", calls)
	}
}

func TestExecScript(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	script := `
CREATE TABLE t (a TEXT, b INTEGER);
INSERT INTO t (a, b) VALUES ('a1', 1);
INSERT INTO t (a, b) VALUES ('a2', 2);
`

	if err := sqlitex.ExecScript(conn, script); err != nil {
		t.Error(err)
	}

	sum := 0
	fn := func(stmt *sqlite.Stmt) error {
		sum = stmt.ColumnInt(0)
		return nil
	}
	if err := sqlitex.Exec(conn, "SELECT sum(b) FROM t;", fn); err != nil {
		t.Fatal(err)
	}

	if sum != 3 {
		t.Errorf("sum=%d, want 3", sum)
	}
}
