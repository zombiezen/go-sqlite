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

package sqlite_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/crawshaw/sqlite"
)

func TestConn(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt, _, err := c.PrepareTransient("CREATE TABLE bartable (foo1 string, foo2 integer);")
	if err != nil {
		t.Fatal(err)
	}
	hasRow, err := stmt.Step()
	if err != nil {
		t.Fatal(err)
	}
	if hasRow {
		t.Errorf("CREATE TABLE reports having a row")
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}

	fooVals := []string{
		"bar",
		"baz",
		"bop",
	}

	for i, val := range fooVals {
		stmt, err := c.Prepare("INSERT INTO bartable (foo1, foo2) VALUES ($f1, $f2);")
		if err != nil {
			t.Fatal(err)
		}
		stmt.SetText("$f1", val)
		stmt.SetInt64("$f2", int64(i))
		hasRow, err = stmt.Step()
		if err != nil {
			t.Errorf("INSERT %q: %v", val, err)
		}
		if hasRow {
			t.Errorf("INSERT %q: has row", val)
		}
	}

	stmt, err = c.Prepare("SELECT foo1, foo2 FROM bartable;")
	if err != nil {
		t.Fatal(err)
	}
	gotVals := []string{}
	gotInts := []int64{}
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			t.Errorf("SELECT: %v", err)
		}
		if !hasRow {
			break
		}
		val := stmt.ColumnText(0)
		if getVal := stmt.GetText("foo1"); getVal != val {
			t.Errorf(`GetText("foo1")=%q, want %q`, getVal, val)
		}
		intVal := stmt.ColumnInt64(1)
		if getIntVal := stmt.GetInt64("foo2"); getIntVal != intVal {
			t.Errorf(`GetText("foo2")=%q, want %q`, getIntVal, intVal)
		}
		gotVals = append(gotVals, val)
		gotInts = append(gotInts, intVal)
	}

	if !reflect.DeepEqual(fooVals, gotVals) {
		t.Errorf("SELECT foo1: got %v, want %v", gotVals, fooVals)
	}

	wantInts := []int64{0, 1, 2}
	if !reflect.DeepEqual(wantInts, gotInts) {
		t.Errorf("SELECT foo2: got %v, want %v", gotInts, wantInts)
	}
}

func TestEarlyInterrupt(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	c.SetInterrupt(ctx.Done())

	stmt, _, err := c.PrepareTransient("CREATE TABLE bartable (foo1 string, foo2 integer);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	stmt.Finalize()

	cancel()

	stmt, err = c.Prepare("INSERT INTO bartable (foo1, foo2) VALUES ($f1, $f2);")
	if err == nil {
		t.Fatal("Prepare err=nil, want prepare to fail")
	}
	if code := sqlite.ErrCode(err); code != sqlite.SQLITE_INTERRUPT {
		t.Fatalf("Prepare err=%s, want SQLITE_INTERRUPT", code)
	}
}

func TestTrailingBytes(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt, trailingBytes, err := conn.PrepareTransient("BEGIN; -- 56")
	if err != nil {
		t.Error(err)
	}
	stmt.Finalize()
	const want = 6
	if trailingBytes != want {
		t.Errorf("trailingBytes=%d, want %d", trailingBytes, want)
	}
}

func TestTrailingBytesError(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	if _, err := conn.Prepare("BEGIN; -- 56"); err == nil {
		t.Error("expecting error on trailing bytes")
	}
}

func TestBadParam(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt, err := c.Prepare("CREATE TABLE IF NOT EXISTS badparam (a, b, c);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}

	stmt, err = c.Prepare("INSERT INTO badparam (a, b, c) VALUES ($a, $b, $c);")
	if err != nil {
		t.Fatal(err)
	}
	stmt.SetText("$a", "col_a")
	stmt.SetText("$b", "col_b")
	stmt.SetText("$badparam", "notaval")
	stmt.SetText("$c", "col_c")
	_, err = stmt.Step()
	if err == nil {
		t.Fatal("expecting error from bad param name, got no error")
	}
	if got := err.Error(); !strings.Contains(got, "$badparam") {
		t.Errorf(`error does not mention "$badparam": %v`, got)
	}
	stmt.Finalize()
}

func TestParallel(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt := c.Prep("CREATE TABLE testparallel (c);")
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}

	stmt = c.Prep("INSERT INTO testparallel (c) VALUES ($c);")
	stmt.SetText("$c", "text")
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	stmt.Reset()
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}

	stmt = c.Prep("SELECT * from testparallel;")
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}

	stmt2 := c.Prep("SELECT count(*) from testparallel;")
	if hasRow, err := stmt2.Step(); err != nil {
		t.Fatal(err)
	} else if !hasRow {
		t.Error("expecting count row")
	}

	if hasRow, err := stmt.Step(); err != nil {
		t.Fatal(err)
	} else if !hasRow {
		t.Error("expecting second row")
	}
}
