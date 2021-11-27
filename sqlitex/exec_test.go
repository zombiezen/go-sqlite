// Copyright (c) 2018 David Crawshaw <david@zentus.com>
// Copyright (c) 2021 Ross Light <rosss@zombiezen.com>
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
//
// SPDX-License-Identifier: ISC

package sqlitex

import (
	"fmt"
	"reflect"
	"testing"

	"zombiezen.com/go/sqlite"
)

func TestExec(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := ExecTransient(conn, "CREATE TABLE t (a TEXT, b INTEGER);", nil); err != nil {
		t.Fatal(err)
	}
	if err := Exec(conn, "INSERT INTO t (a, b) VALUES (?, ?);", nil, "a1", 1); err != nil {
		t.Error(err)
	}
	if err := Exec(conn, "INSERT INTO t (a, b) VALUES (?, ?);", nil, "a2", 2); err != nil {
		t.Error(err)
	}

	var a []string
	var b []int64
	fn := func(stmt *sqlite.Stmt) error {
		a = append(a, stmt.ColumnText(0))
		b = append(b, stmt.ColumnInt64(1))
		return nil
	}
	if err := ExecTransient(conn, "SELECT a, b FROM t;", fn); err != nil {
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

	err = Exec(conn, "INVALID SQL STMT", nil)
	if err == nil {
		t.Error("invalid SQL did not return an error code")
	}
	if got, want := sqlite.ErrCode(err), sqlite.ResultError; got != want {
		t.Errorf("INVALID err code=%s, want %s", got, want)
	}

	if err := Exec(conn, "CREATE TABLE t (c1, c2);", nil); err != nil {
		t.Error(err)
	}
	if err := Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 1, 1); err != nil {
		t.Error(err)
	}
	if err := Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 2, 2); err != nil {
		t.Error(err)
	}
	err = Exec(conn, "INSERT INTO t (c1, c2) VALUES (?, ?);", nil, 1, 1, 1)
	if got, want := sqlite.ErrCode(err), sqlite.ResultRange; got != want {
		t.Errorf("INSERT err code=%s, want %s", got, want)
	}

	calls := 0
	customErr := fmt.Errorf("custom err")
	fn := func(stmt *sqlite.Stmt) error {
		calls++
		return customErr
	}
	err = Exec(conn, "SELECT c1 FROM t;", fn)
	if err != customErr {
		t.Errorf("SELECT want err=customErr, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("SELECT want truncated callback calls, got calls=%d", calls)
	}
}

func TestExecArgsErrors(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	t.Run("TooManyPositional", func(t *testing.T) {
		err := Exec(conn, `SELECT ?;`, nil, 1, 2)
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultRange; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})

	t.Run("Missing", func(t *testing.T) {
		// Compatibility: crawshaw.io/sqlite does not check for this condition.
		err := Exec(conn, `SELECT ?;`, nil)
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultOK; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})
}

func TestExecuteArgsErrors(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	t.Run("TooManyPositional", func(t *testing.T) {
		err := Execute(conn, `SELECT ?;`, &ExecOptions{
			Args: []interface{}{1, 2},
		})
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultRange; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})

	t.Run("ExtraNamed", func(t *testing.T) {
		err := Execute(conn, `SELECT :foo;`, &ExecOptions{
			Named: map[string]interface{}{
				":foo": 42,
				":bar": "hi",
			},
		})
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultRange; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})

	t.Run("Missing", func(t *testing.T) {
		err := Execute(conn, `SELECT ?;`, &ExecOptions{
			Args: []interface{}{},
		})
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultError; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})

	t.Run("MissingNamed", func(t *testing.T) {
		err := Execute(conn, `SELECT :foo;`, nil)
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultError; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})
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

	if err := ExecScript(conn, script); err != nil {
		t.Error(err)
	}

	sum := 0
	fn := func(stmt *sqlite.Stmt) error {
		sum = stmt.ColumnInt(0)
		return nil
	}
	if err := Exec(conn, "SELECT sum(b) FROM t;", fn); err != nil {
		t.Fatal(err)
	}

	if sum != 3 {
		t.Errorf("sum=%d, want 3", sum)
	}
}

func TestExecuteScript(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	script := `
CREATE TABLE t (a TEXT, b INTEGER);
INSERT INTO t (a, b) VALUES ('a1', :a1);
INSERT INTO t (a, b) VALUES ('a2', :a2);
`

	err = ExecuteScript(conn, script, &ExecOptions{
		Named: map[string]interface{}{
			":a1": 1,
			":a2": 2,
		},
	})
	if err != nil {
		t.Error(err)
	}

	sum := 0
	fn := func(stmt *sqlite.Stmt) error {
		sum = stmt.ColumnInt(0)
		return nil
	}
	if err := Exec(conn, "SELECT sum(b) FROM t;", fn); err != nil {
		t.Fatal(err)
	}

	if sum != 3 {
		t.Errorf("sum=%d, want 3", sum)
	}

	t.Run("ExtraNamed", func(t *testing.T) {
		conn, err := sqlite.OpenConn(":memory:", 0)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		script := `
CREATE TABLE t (a TEXT, b INTEGER);
INSERT INTO t (a, b) VALUES ('a1', :a1);
INSERT INTO t (a, b) VALUES ('a2', :a2);
`

		err = ExecuteScript(conn, script, &ExecOptions{
			Named: map[string]interface{}{
				":a1": 1,
				":a2": 2,
				":a3": 3,
			},
		})
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultRange; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})

	t.Run("MissingNamed", func(t *testing.T) {
		conn, err := sqlite.OpenConn(":memory:", 0)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		script := `
CREATE TABLE t (a TEXT, b INTEGER);
INSERT INTO t (a, b) VALUES ('a1', :a1);
INSERT INTO t (a, b) VALUES ('a2', :a2);
`

		err = ExecuteScript(conn, script, &ExecOptions{
			Named: map[string]interface{}{
				":a1": 1,
			},
		})
		t.Log(err)
		if got, want := sqlite.ErrCode(err), sqlite.ResultError; got != want {
			t.Errorf("code = %v; want %v", got, want)
		}
	})
}

func TestBitsetHasAll(t *testing.T) {
	tests := []struct {
		bs   bitset
		n    int
		want bool
	}{
		{
			bs:   bitset{},
			n:    0,
			want: true,
		},
		{
			bs:   bitset{0},
			n:    1,
			want: false,
		},
		{
			bs:   bitset{0x0000000000000001},
			n:    1,
			want: true,
		},
		{
			bs:   bitset{0x8000000000000001},
			n:    1,
			want: true,
		},
		{
			bs:   bitset{0x0000000000000001},
			n:    2,
			want: false,
		},
		{
			bs:   bitset{0xffffffffffffffff},
			n:    64,
			want: true,
		},
		{
			bs:   bitset{0xffffffffffffffff},
			n:    65,
			want: false,
		},
		{
			bs:   bitset{0xffffffffffffffff, 0x0000000000000000},
			n:    65,
			want: false,
		},
		{
			bs:   bitset{0xffffffffffffffff, 0x0000000000000001},
			n:    65,
			want: true,
		},
		{
			bs:   bitset{0x7fffffffffffffff, 0x0000000000000001},
			n:    65,
			want: false,
		},
	}
	for _, test := range tests {
		if got := test.bs.hasAll(test.n); got != test.want {
			t.Errorf("%v.hasAll(%d) = %t; want %t", test.bs, test.n, got, test.want)
		}
	}
}
