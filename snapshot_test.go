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
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

var db = fmt.Sprintf("%v/snapshot_test_%v.sqlite3",
	os.TempDir(), time.Now().UnixNano())

func initDB(t *testing.T) (*sqlite.Conn, *sqlitex.Pool, func()) {
	conn, err := sqlite.OpenConn(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecScript(conn, `CREATE TABLE t (c1 PRIMARY KEY, c2, c3);
                INSERT INTO t (c1, c2, c3) VALUES (1, 2, 3);
                INSERT INTO t (c1, c2, c3) VALUES (2, 4, 5);`); err != nil {
		conn.Close()
		os.Remove(db)
		t.Fatal(err)
	}

	poolFlags := sqlite.SQLITE_OPEN_READONLY |
		sqlite.SQLITE_OPEN_WAL |
		sqlite.SQLITE_OPEN_URI |
		sqlite.SQLITE_OPEN_NOMUTEX
	poolSize := 2
	pool, err := sqlitex.Open(db, poolFlags, poolSize)
	if err != nil {
		conn.Close()
		os.Remove(db)
		t.Fatal(err)
	}
	cleanup := func() {
		pool.Close()
		conn.Close()
		os.Remove(db)
	}

	for i := 0; i < poolSize; i++ {
		c := pool.Get(nil)
		if err := sqlitex.ExecScript(c, `PRAGMA application_id;`); err != nil {
			cleanup()
			t.Fatal(err)
		}
		pool.Put(c)
	}

	return conn, pool, cleanup
}

func TestSnapshot(t *testing.T) {
	conn, pool, cleanup := initDB(t)
	defer cleanup()

	s1, err := pool.GetSnapshot(nil, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := sqlitex.ExecScript(conn, `UPDATE t SET c2 = 100;
                INSERT INTO t (c2, c3) VALUES (2, 3);
                INSERT INTO t (c2, c3) VALUES (2, 3);
                INSERT INTO t (c2, c3) VALUES (2, 3);
                INSERT INTO t (c2, c3) VALUES (2, 3);
                INSERT INTO t (c2, c3) VALUES (2, 3);
                INSERT INTO t (c2, c3) VALUES (2, 3); `); err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Exec(conn, `PRAGMA wal_checkpoint;`, nil); err != nil {
		t.Fatal(err)
	}

	read := pool.Get(nil)
	defer pool.Put(read)

	endRead, err := read.StartSnapshotRead(s1)
	if err != nil {
		t.Fatal(err)
	}

	c2, err := sqlitex.ResultInt(read.Prep(`SELECT c2 FROM t WHERE c1 = 1;`))
	if c2 != 2 {
		t.Fatalf("expected row1 c2 to be 2 but found %v", c2)
	}
	endRead()

	c2, err = sqlitex.ResultInt(read.Prep(`SELECT c2 FROM t WHERE c1 = 1;`))
	if c2 != 100 {
		t.Fatalf("expected row1 c2 to be 100 but found %v", c2)
	}

	s2, release, err := read.GetSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Free()
	defer release()

	if cmp := s1.CompareAges(s2); cmp >= 0 {
		t.Fatalf("expected s1 to be older than s2 but CompareAges returned %v", cmp)
	}

	s1 = nil
	runtime.GC()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	read2 := pool.Get(ctx)
	if read2 == nil {
		t.Fatalf("expected to get another conn from the pool, but it was nil")
	}
	defer pool.Put(read2)
}
