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
	"testing"

	"crawshaw.io/sqlite"
)

func TestFunc(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	xFunc := func(ctx sqlite.Context, values ...sqlite.Value) {
		v := values[0].Int() + values[1].Int()
		ctx.ResultInt(v)
	}
	if err := c.CreateFunction("addints", true, 2, xFunc, nil, nil); err != nil {
		t.Fatal(err)
	}

	stmt, _, err := c.PrepareTransient("SELECT addints(2, 3);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if got, want := stmt.ColumnInt(0), 5; got != want {
		t.Errorf("addints(2, 3)=%d, want %d", got, want)
	}
	stmt.Finalize()
}

func TestAggFunc(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt, _, err := c.PrepareTransient("CREATE TABLE t (c integer);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}

	cVals := []int{3, 5, 7}
	want := 3 + 5 + 7

	stmt, err = c.Prepare("INSERT INTO t (c) VALUES ($c);")
	if err != nil {
		t.Fatal(err)
	}
	for _, val := range cVals {
		stmt.SetInt64("$c", int64(val))
		if _, err = stmt.Step(); err != nil {
			t.Errorf("INSERT %q: %v", val, err)
		}
		if err = stmt.Reset(); err != nil {
			t.Errorf("INSERT reset %q: %v", val, err)
		}
	}
	stmt.Finalize()

	xStep := func(ctx sqlite.Context, values ...sqlite.Value) {
		var sum int
		if data := ctx.UserData(); data != nil {
			sum = data.(int)
		}
		sum += values[0].Int()
		ctx.SetUserData(sum)
	}
	xFinal := func(ctx sqlite.Context) {
		var sum int
		if data := ctx.UserData(); data != nil {
			sum = data.(int)
		}
		ctx.ResultInt(sum)
	}
	if err := c.CreateFunction("sumints", true, 2, nil, xStep, xFinal); err != nil {
		t.Fatal(err)
	}

	stmt, _, err = c.PrepareTransient("SELECT sum(c) FROM t;")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if got := stmt.ColumnInt(0); got != want {
		t.Errorf("sum(c)=%d, want %d", got, want)
	}
	stmt.Finalize()
}
