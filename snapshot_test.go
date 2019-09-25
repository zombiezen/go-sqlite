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
	"fmt"
	"os"
	"testing"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

var db = fmt.Sprintf("%v/snapshot_test_%v.sqlite3",
	os.TempDir(), time.Now().UnixNano())

func initDB(t *testing.T) (*sqlite.Conn, func()) {
	conn, err := sqlite.OpenConn(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		conn.Close()
		os.Remove(db)
	}
	if err := sqlitex.ExecScript(conn, `CREATE TABLE t (c1 PRIMARY KEY, c2, c3);
                INSERT INTO t (c1, c2, c3) VALUES (1, 2, 3);
                INSERT INTO t (c1, c2, c3) VALUES (2, 4, 5);`); err != nil {
		cleanup()
		t.Fatal(err)
	}
	return conn, cleanup
}

func TestSnapshot(t *testing.T) {
	conn, cleanup := initDB(t)
	defer cleanup()

	s1, err := conn.CreateSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Free()

	if err := sqlitex.ExecScript(conn, `UPDATE t SET c2 = 100;`); err != nil {
		t.Fatal(err)
	}

	endRead, err := conn.StartSnapshotRead(s1)
	if err != nil {
		t.Fatal(err)
	}

	c2, err := sqlitex.ResultInt(conn.Prep(`SELECT c2 FROM t WHERE c1 = 1;`))
	if c2 != 2 {
		t.Fatalf("expected row1 c2 to be 2 but found %v", c2)
	}

	endRead()

	c2, err = sqlitex.ResultInt(conn.Prep(`SELECT c2 FROM t WHERE c1 = 1;`))
	if c2 != 100 {
		t.Fatalf("expected row1 c2 to be 100 but found %v", c2)
	}

	s2, err := conn.CreateSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Free()

	if cmp := s1.CompareAges(s2); cmp >= 0 {
		t.Fatalf("expected s1 to be older than s2 but CompareAges returned %v", cmp)
	}
}
