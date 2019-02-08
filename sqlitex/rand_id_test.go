// Copyright (c) 2019 David Crawshaw <david@zentus.com>
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
	"testing"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

func TestRandID(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := sqlitex.ExecTransient(conn, "CREATE TABLE t (key PRIMARY KEY, val TEXT);", nil); err != nil {
		t.Fatal(err)
	}

	stmt := conn.Prep(`INSERT INTO t (key, val) VALUES ($key, $val);`)
	stmt.SetText("$val", "value")

	for i := 0; i < 10; i++ {
		const min, max = 1000, 10000

		id, err := sqlitex.InsertRandID(stmt, "$key", min, max)
		if err != nil {
			t.Fatal(err)
		}
		if id < min || id >= max {
			t.Errorf("id %d out of range [%d, %d)", id, min, max)
		}

		countStmt := conn.Prep("SELECT count(*) FROM t WHERE key = $key;")
		countStmt.SetInt64("$key", id)
		if count, err := sqlitex.ResultInt(countStmt); err != nil {
			t.Fatal(err)
		} else if count != 1 {
			t.Errorf("missing key %d", id)
		}
	}
}
