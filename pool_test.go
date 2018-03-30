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
	"sync"
	"testing"

	"crawshaw.io/sqlite"
)

func TestPool(t *testing.T) {
	const poolSize = 20
	dbpool, err := sqlite.Open("file::memory:?mode=memory", 0, poolSize)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := dbpool.Close(); err != nil {
			t.Error(err)
		}
	}()

	c := dbpool.Get(nil)
	stmt, _, err := c.PrepareTransient("CREATE TABLE footable (col1 integer);")
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
	stmt, _, err = c.PrepareTransient("SELECT COUNT(*) FROM footable;")
	if err != nil {
		t.Fatal(err)
	}
	hasRow, err = stmt.Step()
	if err != nil {
		t.Fatal(err)
	}
	if hasRow {
		count := int(stmt.ColumnInt64(0))
		if want := poolSize * 10 * insertCount; count != want {
			t.Errorf("SELECT COUNT(*) = %d, want %d", count, want)
		}
	} else {
		t.Errorf("SELECT COUNT(*) reports not having a row")
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}
	stmt, _, err = c.PrepareTransient("DROP TABLE footable;")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	dbpool.Put(c)
}

const insertCount = 120

func testInsert(t *testing.T, id string, dbpool *sqlite.Pool) {
	c := dbpool.Get(nil)
	defer dbpool.Put(c)

	begin, _, err := c.PrepareTransient("BEGIN;")
	if err != nil {
		t.Errorf("id=%s: BEGIN: %v", id, err)
	}
	commit, _, err := c.PrepareTransient("COMMIT;")
	if err != nil {
		t.Errorf("id=%s: COMMIT: %v", id, err)
	}
	stmt, err := c.Prepare("INSERT INTO footable (col1) VALUES (?);")
	if err != nil {
		t.Errorf("id=%s: prepare: %v", id, err)
	}

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

	begin.Finalize()
	commit.Finalize()
}
