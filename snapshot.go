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

package sqlite

// #include <sqlite3.h>
// #include <stdlib.h>
import "C"
import (
	"runtime"
	"unsafe"
)

// A Snapshot records the state of a WAL mode database for some specific point
// in history.
//
// Equivalent to the sqlite3_snapshot* C object.
//
// https://www.sqlite.org/c3ref/snapshot.html
type Snapshot struct {
	ptr    *C.sqlite3_snapshot
	schema *C.char
}

// CreateSnapshot attempts to make a new Snapshot that records the current state
// of schema in conn.
//
// The following must be true for this function to succeed:
//
// - The schema of conn must be a WAL mode database.
//
// - There must not be any transaction open on schema of conn.
//
// - At least one transaction must have been written to the current WAL file
// since it was created on disk (by any connection). You can create and drop an
// empty table to achieve this.
//
// https://www.sqlite.org/c3ref/snapshot_get.html
func (conn *Conn) CreateSnapshot(schema string) (*Snapshot, error) {
	var s Snapshot
	if schema == "" || schema == "main" {
		s.schema = cmain
	} else {
		s.schema = C.CString(schema)
		defer C.free(unsafe.Pointer(s.schema))
	}

	commit := conn.disableAutoCommitMode()
	defer commit()

	res := C.sqlite3_snapshot_get(conn.conn, s.schema, &s.ptr)
	if res == 0 {
		runtime.SetFinalizer(&s, func(s *Snapshot) {
			if s.ptr != nil {
				panic("open *sqlite.Snapshot garbage collected, call Free method")
			}
		})
	}

	return &s, reserr("Conn.CreateSnapshot", "", "", res)
}

// Free destroys a Snapshot. The application must eventually free every Snapshot
// to avoid a memory leak.
//
// https://www.sqlite.org/c3ref/snapshot_free.html
func (s *Snapshot) Free() {
	C.sqlite3_snapshot_free(s.ptr)
	s.ptr = nil
}

// CompareAges returns whether s1 is older, newer or the same age as s2. Age
// refers to writes on the database, not time since creation.
//
// If s is older than s2, a negative number is returned. If s and s2 are the
// same age, zero is returned. If s is newer than s2, a positive number is
// returned.
//
// The result is valid only if both of the following are true:
//
// - The two snapshot handles are associated with the same database file.
// 
// - Both of the Snapshots were obtained since the last time the wal file was
// deleted.
//
// https://www.sqlite.org/c3ref/snapshot_cmp.html
func (s *Snapshot) CompareAges(s2 *Snapshot) int {
	return int(C.sqlite3_snapshot_cmp(s.ptr, s2.ptr))
}

// StartSnapshotRead starts a new read transaction on conn such that the read
// transaction refers to historical Snapshot s, rather than the most recent
// change to the database.
//
// There must be no open transaction on conn.
//
// If err is nil, then endRead is a function that will end the read transaction
// and return conn to its original state. Until endRead is called, no writes may
// occur on conn, and all reads on conn will refer to the Snapshot.
//
// https://www.sqlite.org/c3ref/snapshot_open.html
func (conn *Conn) StartSnapshotRead(s *Snapshot) (endRead func(), err error) {
	endRead = conn.disableAutoCommitMode()
	res := C.sqlite3_snapshot_open(conn.conn, s.schema, s.ptr)
	if res != 0 {
		endRead()
		return nil, reserr("Conn.StartSnapshotRead", "", "", res)
	}

	return endRead, nil
}

// disableAutoCommitMode starts a read transaction with `BEGIN;`, disabling
// autocommit mode, and returns a function which when called will end the read
// transaction with `COMMIT;`, re-enabling autocommit mode.
//
// https://sqlite.org/c3ref/get_autocommit.html
func (conn *Conn) disableAutoCommitMode() func() {
	begin, _, err := conn.PrepareTransient("BEGIN;")
	if err != nil {
		panic(err)
	}
	defer begin.Finalize()
	if _, err := begin.Step(); err != nil {
		panic(err)
	}
	return func() {
		commit, _, err := conn.PrepareTransient("COMMIT;")
		if err != nil {
			panic(err)
		}
		defer commit.Finalize()
		if _, err := commit.Step(); err != nil {
			panic(err)
		}
	}
}
