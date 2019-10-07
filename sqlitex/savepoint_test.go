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

package sqlitex

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crawshaw.io/sqlite"
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

		if err := Exec(conn, `INSERT INTO t VALUES ('hello');`, nil); err != nil {
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
	if err := Exec(conn, `INSERT INTO t VALUES ('one');`, nil); err != nil {
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

	if err := Exec(conn, `INSERT INTO t VALUES ('hello');`, nil); err != nil {
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

func TestReleaseTx(t *testing.T) {
	conn1, err := sqlite.OpenConn("file::memory:?mode=memory&cache=shared", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()

	conn2, err := sqlite.OpenConn("file::memory:?mode=memory&cache=shared", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	Exec(conn1, "DROP TABLE t;", nil)
	if err := Exec(conn1, "CREATE TABLE t (c1);", nil); err != nil {
		t.Fatal(err)
	}
	countFn := func() int {
		var count int
		fn := func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		}
		if err := Exec(conn2, "SELECT count(*) FROM t;", fn); err != nil {
			t.Fatal(err)
		}
		return count
	}
	errNoSuccess := errors.New("succeed=false")
	insert := func(succeed bool) (err error) {
		defer Save(conn1)(&err)

		if err := Exec(conn1, `INSERT INTO t VALUES ('hello');`, nil); err != nil {
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

	if err := insert(false); err == nil {
		t.Fatal(err)
	}
	// If the transaction is still open, countFn will get stuck
	// on conn2 waiting for conn1's write lock to release.
	if got := countFn(); got != 1 {
		t.Errorf("expecting 1 row, got %d", got)
	}
}

func TestInterruptRollback(t *testing.T) {
	conn, err := sqlite.OpenConn("file::memory:?mode=memory&cache=shared", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err = ExecScript(conn, `
		DROP TABLE IF EXISTS t;
		CREATE TABLE t (c);
	`); err != nil {
		t.Fatal(err)
	}

	releaseFn := Save(conn)
	if err := Exec(conn, `INSERT INTO t (c) VALUES (1);`, nil); err != nil {
		t.Fatal(err)
	}
	releaseFn(&err)
	if err != nil {
		t.Fatalf("relaseFn err: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	conn.SetInterrupt(ctx.Done())

	releaseFn1 := Save(conn)
	if err := Exec(conn, `INSERT INTO t (c) VALUES (2);`, nil); err != nil {
		t.Fatal(err)
	}
	releaseFn2 := Save(conn)
	if err := Exec(conn, `INSERT INTO t (c) VALUES (3);`, nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := Exec(conn, `INSERT INTO t (c) VALUES (3);`, nil); err == nil || sqlite.ErrCode(err) != sqlite.SQLITE_INTERRUPT {
		t.Fatalf("want SQLITE_INTERRUPT, got %v", err)
	}
	err = context.Canceled
	releaseFn2(&err) // given a real error, should rollback
	if err != context.Canceled {
		t.Fatalf("relaseFn2 err: %v", err)
	}
	var errNil error
	releaseFn1(&errNil) // given no error, but we are interrupted, so should rollback
	if errNil == nil {
		t.Fatal("relaseFn1 errNil is still nil, should have been set to interrupt")
	}
	if sqlite.ErrCode(errNil) != sqlite.SQLITE_INTERRUPT {
		t.Fatalf("relaseFn1 errNil=%v, want SQLITE_INTERRUPT", errNil)
	}

	conn.SetInterrupt(nil)
	got, err := ResultInt(conn.Prep("SELECT count(*) FROM t;"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("want 1 row, got %d", got)
	}
}

var veryLongScript = `
drop table if exists naturals;
create table naturals
( n integer unique primary key asc,
  isprime bool,
  factor integer);

with recursive
  nn (n)
as (
  select 2
  union all
  select n+1 as newn from nn
  where newn < 1e10
)
insert into naturals
select n, 1, null from nn;

insert or replace into naturals
  with recursive
    product (prime,composite)
  as (
    select n, n*n as sqr
      from naturals
      where sqr <= (select max(n) from naturals)
    union all
    select prime, composite+prime as prod
    from
      product
    where
      prod <= (select max(n) from naturals)
  )
select n, 0, prime
from product join naturals
  on (product.composite = naturals.n)
;
`

func TestInterruptRollbackLongQuery(t *testing.T) {
	conn, err := sqlite.OpenConn("file::memory:?mode=memory&cache=shared", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err = ExecScript(conn, `
		DROP TABLE IF EXISTS t;
		CREATE TABLE t (c);
	`); err != nil {
		t.Fatal(err)
	}

	releaseFn := Save(conn)
	if err := Exec(conn, `INSERT INTO t (c) VALUES (1);`, nil); err != nil {
		t.Fatal(err)
	}
	releaseFn(&err)
	if err != nil {
		t.Fatalf("relaseFn err: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	conn.SetInterrupt(ctx.Done())
	testDone := make(chan struct{})
	go func() {
		defer close(testDone)
		defer func() {
			if err := recover(); err != nil {
				t.Log("a panic occurred during rollback\n", err)
				t.Fail()
			}
		}()
		defer Save(conn)(&err)
		if err := Exec(conn, `INSERT INTO t (c) VALUES (3);`, nil); err != nil {
			t.Fatalf("interrupted too early")
		}
		err = ExecScript(conn, veryLongScript)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-testDone
	conn.SetInterrupt(nil)
	got, err := ResultInt(conn.Prep("SELECT count(*) FROM t;"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("want 1 row, got %d", got)
	}
}

func TestBusySnapshot(t *testing.T) {
	dir, err := ioutil.TempDir("", "sqlitex-test-")
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "busysnapshot.db")

	// No SQLITE_OPEN_SHAREDCACHE.
	flags := sqlite.SQLITE_OPEN_READWRITE | sqlite.SQLITE_OPEN_CREATE | sqlite.SQLITE_OPEN_WAL
	conn0, err := sqlite.OpenConn(db, flags)
	if err != nil {
		t.Fatal(err)
	}
	defer conn0.Close()

	conn1, err := sqlite.OpenConn(db, flags)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()

	if err = ExecScript(conn0, `
		DROP TABLE IF EXISTS t;
		CREATE TABLE t (c, b BLOB);
		INSERT INTO t (c, b) VALUES (4, 'hi');
	`); err != nil {
		t.Fatal(err)
	}

	conn0Release0 := Save(conn0)

	c, err := ResultInt(conn0.Prep("SELECT count(*) FROM t WHERE c > 3;"))
	if err != nil {
		t.Fatal(err)
	}

	// An insert on conn1 invalidates the deferred transaction on conn0.
	if err := Exec(conn1, "INSERT INTO t (c) VALUES (4);", nil); err != nil {
		t.Fatal(err)
	}

	stmt := conn0.Prep("UPDATE t SET c = $c WHERE c = 4;")
	stmt.SetInt64("$c", int64(c))
	_, conn0Err := stmt.Step()
	if sqlite.ErrCode(conn0Err) != sqlite.SQLITE_BUSY_SNAPSHOT {
		t.Fatalf("want SQLITE_BUSY_SNAPSHOT, got: %v", conn0Err)
	}

	conn0Release0(&conn0Err)
	if sqlite.ErrCode(conn0Err) != sqlite.SQLITE_BUSY_SNAPSHOT {
		t.Fatalf("after savepoint want SQLITE_BUSY_SNAPSHOT, got: %v", conn0Err)
	}
}
