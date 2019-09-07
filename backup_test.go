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
	"crawshaw.io/sqlite/sqlitex"
)

func initSrc(t *testing.T) *sqlite.Conn {
	conn, err := sqlite.OpenConn(`:memory:`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecScript(conn, `CREATE TABLE t (c1 PRIMARY KEY, c2, c3);
                INSERT INTO t (c1, c2, c3) VALUES (1, 2, 3);
                INSERT INTO t (c1, c2, c3) VALUES (2, 4, 5);`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}

func TestBackup(t *testing.T) {
	conn := initSrc(t)
	defer conn.Close()

	copyConn, err := conn.BackupToDB("", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer copyConn.Close()

	count, err := sqlitex.ResultInt(copyConn.Prep(`SELECT count(*) FROM t;`))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows but found %v", count)
	}
	c2, err := sqlitex.ResultInt(copyConn.Prep(`SELECT c2 FROM t WHERE c1 = 1;`))
	if c2 != 2 {
		t.Fatalf("expected row1 c2 to be 2 but found %v", c2)
	}

	c2, err = sqlitex.ResultInt(copyConn.Prep(`SELECT c2 FROM t WHERE c1 = 2;`))
	if c2 != 4 {
		t.Fatalf("expected row2 c2 to be 4 but found %v", c2)
	}
}
