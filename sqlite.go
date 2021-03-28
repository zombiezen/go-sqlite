// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

import (
	"fmt"
	"strings"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	lib "modernc.org/sqlite/lib"
)

// Conn is an open connection to an SQLite3 database.
//
// A Conn can only be used by goroutine at a time.
type Conn struct {
	tls    *libc.TLS
	conn   uintptr
	closed bool
}

// OpenFlags are flags used when opening a Conn.
//
// https://www.sqlite.org/c3ref/c_open_autoproxy.html
type OpenFlags int

const (
	OpenReadOnly      = OpenFlags(lib.SQLITE_OPEN_READONLY)
	OpenReadWrite     = OpenFlags(lib.SQLITE_OPEN_READWRITE)
	OpenCreate        = OpenFlags(lib.SQLITE_OPEN_CREATE)
	OpenURI           = OpenFlags(lib.SQLITE_OPEN_URI)
	OpenMemory        = OpenFlags(lib.SQLITE_OPEN_MEMORY)
	OpenMainDB        = OpenFlags(lib.SQLITE_OPEN_MAIN_DB)
	OpenTempDB        = OpenFlags(lib.SQLITE_OPEN_TEMP_DB)
	OpenTransientDB   = OpenFlags(lib.SQLITE_OPEN_TRANSIENT_DB)
	OpenMainJournal   = OpenFlags(lib.SQLITE_OPEN_MAIN_JOURNAL)
	OpenTempJournal   = OpenFlags(lib.SQLITE_OPEN_TEMP_JOURNAL)
	OpenSubjournal    = OpenFlags(lib.SQLITE_OPEN_SUBJOURNAL)
	OpenMasterJournal = OpenFlags(lib.SQLITE_OPEN_MASTER_JOURNAL)
	OpenNoMutex       = OpenFlags(lib.SQLITE_OPEN_NOMUTEX)
	OpenFullMutex     = OpenFlags(lib.SQLITE_OPEN_FULLMUTEX)
	OpenSharedCache   = OpenFlags(lib.SQLITE_OPEN_SHAREDCACHE)
	OpenPrivateCache  = OpenFlags(lib.SQLITE_OPEN_PRIVATECACHE)
	OpenWAL           = OpenFlags(lib.SQLITE_OPEN_WAL)
)

const ptrSize = types.Size_t(unsafe.Sizeof(uintptr(0)))

func OpenConn(path string, flags OpenFlags) (*Conn, error) {
	tls := libc.NewTLS()
	cpath, err := libc.CString(path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	defer libc.Xfree(tls, cpath)
	connPtr, err := malloc(tls, ptrSize)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	defer libc.Xfree(tls, connPtr)
	res := ResultCode(lib.Xsqlite3_open_v2(tls, cpath, connPtr, int32(flags), 0))
	c := &Conn{
		tls:  tls,
		conn: *(*uintptr)(unsafe.Pointer(connPtr)),
	}
	if c.conn == 0 {
		// Not enough memory to allocate the sqlite3 object.
		return nil, fmt.Errorf("sqlite: open %q: %w", path, sqliteError{res})
	}
	if err := c.lastError(res); err != nil {
		// sqlite3_open_v2 may still return a sqlite3* just so we can extract the error.
		c.Close()
		return nil, fmt.Errorf("sqlite: open %q: %w", path, c.lastError(res))
	}
	return c, nil
}

// Close closes the database connection using sqlite3_close and finalizes
// persistent prepared statements. https://www.sqlite.org/c3ref/close.html
func (c *Conn) Close() error {
	c.closed = true
	if res := ResultCode(lib.Xsqlite3_close(c.tls, c.conn)); !res.IsSuccess() {
		return fmt.Errorf("sqlite: close: %w", sqliteError{res})
	}
	return nil
}

func (c *Conn) lastError(res ResultCode) error {
	if res.IsSuccess() {
		return nil
	}
	return sqliteError{
		code: ResultCode(lib.Xsqlite3_extended_errcode(c.tls, c.conn)),
	}
}

func (c *Conn) PrepareTransient(query string) (stmt *Stmt, trailingBytes int, err error) {
	return c.prepare(query, 0)
}

func (c *Conn) prepare(query string, flags uint32) (*Stmt, int, error) {
	cquery, err := libc.CString(query)
	if err != nil {
		return nil, 0, err
	}
	defer libc.Xfree(c.tls, cquery)
	ctrailingPtr, err := malloc(c.tls, ptrSize)
	if err != nil {
		return nil, 0, err
	}
	defer libc.Xfree(c.tls, ctrailingPtr)
	stmtPtr, err := malloc(c.tls, ptrSize)
	if err != nil {
		return nil, 0, err
	}
	defer libc.Xfree(c.tls, stmtPtr)
	res := ResultCode(lib.Xsqlite3_prepare_v3(c.tls, c.conn, cquery, -1, flags, stmtPtr, ctrailingPtr))
	if err := c.lastError(res); err != nil {
		return nil, 0, err
	}
	ctrailing := *(*uintptr)(unsafe.Pointer(ctrailingPtr))
	trailingBytes := len(query) - int(ctrailing-cquery)
	// TODO(now): Bind parameter names and column names.
	return &Stmt{
		conn:  c,
		query: query,
		stmt:  *(*uintptr)(unsafe.Pointer(stmtPtr)),
	}, trailingBytes, nil
}

type Stmt struct {
	conn  *Conn
	query string
	stmt  uintptr
}

// Finalize deletes a prepared statement.
//
// Be sure to always call Finalize when done with
// a statement created using PrepareTransient.
//
// Do not call Finalize on a prepared statement that
// you intend to prepare again in the future.
//
// https://www.sqlite.org/c3ref/finalize.html
func (stmt *Stmt) Finalize() error {
	res := ResultCode(lib.Xsqlite3_finalize(stmt.conn.tls, stmt.stmt))
	err := stmt.conn.lastError(res)
	stmt.conn = nil
	if err != nil {
		return fmt.Errorf("sqlite: finalize: %w", err)
	}
	return nil
}

func (stmt *Stmt) Step() (rowReturned bool, err error) {
	rowReturned, err = stmt.step()
	return
}

func (stmt *Stmt) step() (bool, error) {
	for {
		switch res := ResultCode(lib.Xsqlite3_step(stmt.conn.tls, stmt.stmt)); res.ToPrimary() {
		case ResultLocked:
			// loop
		case ResultRow:
			return true, nil
		case ResultDone:
			return false, nil
		default:
			return false, fmt.Errorf("sqlite: step: %w", stmt.conn.lastError(res))
		}
	}
}

// ColumnText returns a query result as a string.
//
// Column indices start at 0.
//
// https://www.sqlite.org/c3ref/column_blob.html
func (stmt *Stmt) ColumnText(col int) string {
	n := stmt.ColumnLen(col)
	cstr := lib.Xsqlite3_column_text(stmt.conn.tls, stmt.stmt, int32(col))
	return goStringN(cstr, n)
}

// ColumnLen returns the number of bytes in a query result.
//
// Column indices start at 0.
//
// https://www.sqlite.org/c3ref/column_blob.html
func (stmt *Stmt) ColumnLen(col int) int {
	return int(lib.Xsqlite3_column_bytes(stmt.conn.tls, stmt.stmt, int32(col)))
}

func malloc(tls *libc.TLS, n types.Size_t) (uintptr, error) {
	p := libc.Xmalloc(tls, n)
	if p == 0 {
		return 0, fmt.Errorf("out of memory")
	}
	return p, nil
}

func goStringN(s uintptr, n int) string {
	if s == 0 {
		return ""
	}
	var buf strings.Builder
	buf.Grow(n)
	for i := 0; i < n; i++ {
		buf.WriteByte(*(*byte)(unsafe.Pointer(s)))
		s++
	}
	return buf.String()
}
