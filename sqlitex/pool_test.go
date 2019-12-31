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
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

const poolSize = 20

// newMemPool returns new sqlitex.Pool attached to new database opened in memory.
//
// the pool is initialized with size=poolSize.
// any error is t.Fatal.
func newMemPool(t *testing.T) *sqlitex.Pool {
	t.Helper()
	flags := sqlite.SQLITE_OPEN_READWRITE | sqlite.SQLITE_OPEN_CREATE | sqlite.SQLITE_OPEN_URI | sqlite.SQLITE_OPEN_NOMUTEX | sqlite.SQLITE_OPEN_SHAREDCACHE
	dbpool, err := sqlitex.Open("file::memory:?mode=memory&cache=shared", flags, poolSize)
	if err != nil {
		t.Fatal(err)
	}
	return dbpool
}

func TestPool(t *testing.T) {
	dbpool := newMemPool(t)
	defer func() {
		if err := dbpool.Close(); err != nil {
			t.Error(err)
		}
	}()

	c := dbpool.Get(nil)
	c.Prep("DROP TABLE IF EXISTS footable;").Step()
	if hasRow, err := c.Prep("CREATE TABLE footable (col1 integer);").Step(); err != nil {
		t.Fatal(err)
	} else if hasRow {
		t.Errorf("CREATE TABLE reports having a row")
	}
	dbpool.Put(c)
	c = nil

	var wg sync.WaitGroup
	for i := 0; i < poolSize; i++ {
		wg.Add(1)
		go func(i int) {
			for j := 0; j < 10; j++ {
				testInsert(t, fmt.Sprintf("%d-%d", i, j), dbpool)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	c = dbpool.Get(nil)
	defer dbpool.Put(c)
	stmt := c.Prep("SELECT COUNT(*) FROM footable;")
	if hasRow, err := stmt.Step(); err != nil {
		t.Fatal(err)
	} else if hasRow {
		count := int(stmt.ColumnInt64(0))
		if want := poolSize * 10 * insertCount; count != want {
			t.Errorf("SELECT COUNT(*) = %d, want %d", count, want)
		}
	} else {
		t.Errorf("SELECT COUNT(*) reports not having a row")
	}
	stmt.Reset()
}

const insertCount = 120

func testInsert(t *testing.T, id string, dbpool *sqlitex.Pool) {
	c := dbpool.Get(nil)
	defer dbpool.Put(c)

	begin := c.Prep("BEGIN;")
	commit := c.Prep("COMMIT;")
	stmt := c.Prep("INSERT INTO footable (col1) VALUES (?);")

	if _, err := begin.Step(); err != nil {
		t.Errorf("id=%s: BEGIN step: %v", id, err)
	}
	for i := int64(0); i < insertCount; i++ {
		if err := stmt.Reset(); err != nil {
			t.Errorf("id=%s: reset: %v", id, err)
		}
		stmt.BindInt64(1, i)
		if _, err := stmt.Step(); err != nil {
			t.Errorf("id=%s: step: %v", id, err)
		}
	}
	if _, err := commit.Step(); err != nil {
		t.Errorf("id=%s: COMMIT step: %v", id, err)
	}
}

func TestPoolAfterClose(t *testing.T) {
	// verify that Get after close never try to initialize a Conn and segfault
	dbpool := newMemPool(t)

	err := dbpool.Close()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10*poolSize; i++ {
		conn := dbpool.Get(nil)
		if conn != nil {
			t.Fatal("dbpool: Get after Close -> !nil conn")
		}
	}
}

func TestSharedCacheLock(t *testing.T) {
	dir, err := ioutil.TempDir("", "sqlite-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	dbFile := filepath.Join(dir, "awal.db")

	c0, err := sqlite.OpenConn(dbFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c0.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = sqlitex.ExecScript(c0, `
		DROP TABLE IF EXISTS t;
		CREATE TABLE t (c, content BLOB);
		DROP TABLE IF EXISTS t2;
		CREATE TABLE t2 (c);
		INSERT INTO t2 (c) VALUES ('hello');
		`)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := sqlite.OpenConn(dbFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c1.Close(); err != nil {
			t.Error(err)
		}
	}()

	c0Lock := func() {
		if _, err := c0.Prep("BEGIN;").Step(); err != nil {
			t.Fatal(err)
		}
		if _, err := c0.Prep("INSERT INTO t (c, content) VALUES (0, 'hi');").Step(); err != nil {
			t.Fatal(err)
		}
	}
	c0Unlock := func() {
		if err := sqlitex.Exec(c0, "COMMIT;", nil); err != nil {
			t.Fatal(err)
		}
	}

	c0Lock()

	stmt := c1.Prep("INSERT INTO t (c) VALUES (1);")

	done := make(chan struct{})
	go func() {
		if _, err := stmt.Step(); err != nil {
			t.Fatal(err)
		}
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	select {
	case <-done:
		t.Error("insert done while transaction was held")
	default:
	}

	c0Unlock()

	// End the initial transaction, allowing the goroutine to complete
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("second connection insert not completing")
	}

	// TODO: It is possible for stmt.Reset to return SQLITE_LOCKED.
	//       Work out why and find a way to test it.
}

func TestPoolPutMatch(t *testing.T) {
	dbpool0 := newMemPool(t)
	dbpool1 := newMemPool(t)
	defer func() {
		if err := dbpool0.Close(); err != nil {
			t.Error(err)
		}
		if err := dbpool1.Close(); err != nil {
			t.Error(err)
		}
	}()

	func() {
		c := dbpool0.Get(nil)
		defer func() {
			if r := recover(); r == nil {
				t.Error("expect put mismatch panic, got none")
			}
			dbpool0.Put(c)
		}()

		dbpool1.Put(c)
	}()
}
