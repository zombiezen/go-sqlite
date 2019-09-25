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
	"bytes"
	"reflect"
	"testing"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

func initT(t *testing.T, conn *sqlite.Conn) {
	if _, err := conn.Prep(`INSERT INTO t (c1, c2, c3) VALUES ('1', '2', '3');`).Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Prep(`INSERT INTO t (c1, c2, c3) VALUES ('4', '5', '6');`).Step(); err != nil {
		t.Fatal(err)
	}
}

func fillSession(t *testing.T) (*sqlite.Conn, *sqlite.Session) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Prep("CREATE TABLE t (c1 PRIMARY KEY, c2, c3);").Step(); err != nil {
		t.Fatal(err)
	}
	initT(t, conn) // two rows that predate the session

	s, err := conn.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(""); err != nil {
		t.Fatal(err)
	}

	stmts := []string{
		`UPDATE t SET c1='one' WHERE c1='1';`,
		`UPDATE t SET c2='two', c3='three' WHERE c1='one';`,
		`UPDATE t SET c1='noop' WHERE c2='2';`,
		`DELETE FROM t WHERE c1='4';`,
		`INSERT INTO t (c1, c2, c3) VALUES ('four', 'five', 'six');`,
	}

	for _, stmt := range stmts {
		if _, err := conn.Prep(stmt).Step(); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := conn.Prep("BEGIN;").Step(); err != nil {
		t.Fatal(err)
	}
	stmt, err := conn.Prepare("INSERT INTO t (c1, c2, c3) VALUES (?,?,?);")
	if err != nil {
		t.Fatal(err)
	}
	for i := int64(2); i < 100; i++ {
		stmt.Reset()
		stmt.BindInt64(1, i)
		stmt.BindText(2, "column2")
		stmt.BindText(3, "column3")
		if _, err := stmt.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Prep("COMMIT;").Step(); err != nil {
		t.Fatal(err)
	}

	return conn, s
}

func TestFillSession(t *testing.T) {
	conn, s := fillSession(t)
	s.Delete()
	conn.Close()
}

func TestChangeset(t *testing.T) {
	conn, s := fillSession(t)
	defer func() {
		s.Delete()
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	buf := new(bytes.Buffer)
	if err := s.Changeset(buf); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if len(b) == 0 {
		t.Errorf("changeset has no length")
	}

	iter, err := sqlite.ChangesetIterStart(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	numChanges := 0
	num3Cols := 0
	opTypes := make(map[sqlite.OpType]int)
	for {
		hasRow, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !hasRow {
			break
		}
		table, numCols, opType, _, err := iter.Op()
		if err != nil {
			t.Fatalf("numChanges=%d, Op err: %v", numChanges, err)
		}
		if table != "t" {
			t.Errorf("table=%q, want t", table)
		}
		opTypes[opType]++
		if numCols == 3 {
			num3Cols++
		}
		numChanges++
	}
	if numChanges != 102 {
		t.Errorf("numChanges=%d, want 102", numChanges)
	}
	if num3Cols != 102 {
		t.Errorf("num3Cols=%d, want 102", num3Cols)
	}
	if got := opTypes[sqlite.SQLITE_INSERT]; got != 100 {
		t.Errorf("num inserts=%d, want 100", got)
	}
	if err := iter.Finalize(); err != nil {
		t.Fatal(err)
	}
}

func TestChangesetInvert(t *testing.T) {
	conn, s := fillSession(t)
	defer func() {
		s.Delete()
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	buf := new(bytes.Buffer)
	if err := s.Changeset(buf); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()

	buf = new(bytes.Buffer)
	if err := sqlite.ChangesetInvert(buf, bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	invB := buf.Bytes()
	if len(invB) == 0 {
		t.Error("no inverted changeset")
	}
	if bytes.Equal(b, invB) {
		t.Error("inverted changeset is unchanged")
	}

	buf = new(bytes.Buffer)
	if err := sqlite.ChangesetInvert(buf, bytes.NewReader(invB)); err != nil {
		t.Fatal(err)
	}
	invinvB := buf.Bytes()
	if !bytes.Equal(b, invinvB) {
		t.Error("inv(inv(b)) != b")
	}
}

func TestChangesetApply(t *testing.T) {
	conn, s := fillSession(t)
	defer func() {
		s.Delete()
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	buf := new(bytes.Buffer)
	if err := s.Changeset(buf); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()

	invBuf := new(bytes.Buffer)
	if err := sqlite.ChangesetInvert(invBuf, bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}

	// Undo the entire session.
	if err := conn.ChangesetApply(invBuf, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Table t should now be equivalent to the first two statements:
	//	INSERT INTO t (c1, c2, c3) VALUES ("1", "2", "3");
	//	INSERT INTO t (c1, c2, c3) VALUES ("4", "5", "6");
	want := []string{"1,2,3", "4,5,6"}
	var got []string
	fn := func(stmt *sqlite.Stmt) error {
		got = append(got, stmt.ColumnText(0)+","+stmt.ColumnText(1)+","+stmt.ColumnText(2))
		return nil
	}
	if err := sqlitex.Exec(conn, "SELECT c1, c2, c3 FROM t ORDER BY c1;", fn); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v, want=%v", got, want)
	}
}

func TestPatchsetApply(t *testing.T) {
	conn, s := fillSession(t)
	defer func() {
		if s != nil {
			s.Delete()
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	var rowCountBefore int
	fn := func(stmt *sqlite.Stmt) error {
		rowCountBefore = stmt.ColumnInt(0)
		return nil
	}
	if err := sqlitex.Exec(conn, "SELECT COUNT(*) FROM t;", fn); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	if err := s.Patchset(buf); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()

	s.Delete()
	s = nil

	if _, err := conn.Prep("DELETE FROM t;").Step(); err != nil {
		t.Fatal(err)
	}
	initT(t, conn)

	filterFnCalled := false
	filterFn := func(tableName string) bool {
		if tableName == "t" {
			filterFnCalled = true
			return true
		} else {
			t.Errorf("unexpected table in filter fn: %q", tableName)
			return false
		}
	}
	conflictFn := func(sqlite.ConflictType, sqlite.ChangesetIter) sqlite.ConflictAction {
		t.Error("conflict applying patchset")
		return sqlite.SQLITE_CHANGESET_ABORT
	}
	if err := conn.ChangesetApply(bytes.NewReader(b), filterFn, conflictFn); err != nil {
		t.Fatal(err)
	}
	if !filterFnCalled {
		t.Error("filter function not called")
	}

	var rowCountAfter int
	fn = func(stmt *sqlite.Stmt) error {
		rowCountAfter = stmt.ColumnInt(0)
		return nil
	}
	if err := sqlitex.Exec(conn, "SELECT COUNT(*) FROM t;", fn); err != nil {
		t.Fatal(err)
	}

	if rowCountBefore != rowCountAfter {
		t.Errorf("row count is %d, want %d", rowCountAfter, rowCountBefore)
	}

	// Second application of patchset should fail.
	haveConflict := false
	conflictFn = func(ct sqlite.ConflictType, iter sqlite.ChangesetIter) sqlite.ConflictAction {
		if ct == sqlite.SQLITE_CHANGESET_CONFLICT {
			haveConflict = true
		} else {
			t.Errorf("unexpected conflict type: %v", ct)
		}
		_, _, opType, _, err := iter.Op()
		if err != nil {
			t.Errorf("conflict iter.Op() error: %v", err)
		}
		if opType != sqlite.SQLITE_INSERT {
			t.Errorf("unexpected conflict op type: %v", opType)
		}
		return sqlite.SQLITE_CHANGESET_ABORT
	}
	err := conn.ChangesetApply(bytes.NewReader(b), nil, conflictFn)
	if code := sqlite.ErrCode(err); code != sqlite.SQLITE_ABORT {
		t.Errorf("conflicting changeset Apply error is %v, want SQLITE_ABORT", err)
	}
	if !haveConflict {
		t.Error("no conflict found")
	}
}
