// Copyright (c) 2018 David Crawshaw <david@zentus.com>
// Copyright (c) 2021 Ross Light <ross@zombiezen.com>
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
//
// SPDX-License-Identifier: ISC

package sqlite

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	// The pointer operations for Read and Write assume a non-moving GC.
	_ "go4.org/unsafe/assume-no-moving-gc"
	"modernc.org/libc"
	lib "modernc.org/sqlite/lib"
)

var (
	mainCString = mustCString("main")
	tempCString = mustCString("temp")
)

// OpenBlob opens a blob in a particular {database,table,column,row}.
//
// https://www.sqlite.org/c3ref/blob_open.html
func (conn *Conn) OpenBlob(dbn, table, column string, row int64, write bool) (*Blob, error) {
	var cdb uintptr
	switch dbn {
	case "", "main":
		cdb = mainCString
	case "temp":
		cdb = tempCString
	default:
		var err error
		cdb, err = libc.CString(dbn)
		if err != nil {
			return nil, fmt.Errorf("sqlite: open blob %q.%q blob: %w", table, column, err)
		}
		defer libc.Xfree(conn.tls, cdb)
	}
	var writeFlag int32
	if write {
		writeFlag = 1
	}

	ctable, err := libc.CString(table)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open blob %q.%q blob: %w", table, column, err)
	}
	defer libc.Xfree(conn.tls, ctable)
	ccolumn, err := libc.CString(column)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open blob %q.%q blob: %w", table, column, err)
	}
	defer libc.Xfree(conn.tls, ccolumn)

	blobPtrPtr, err := malloc(conn.tls, ptrSize)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open blob %q.%q blob: %w", table, column, err)
	}
	defer libc.Xfree(conn.tls, blobPtrPtr)
	for {
		if err := conn.interrupted(); err != nil {
			return nil, fmt.Errorf("sqlite: open blob %q.%q blob: %w", table, column, err)
		}
		res := ResultCode(lib.Xsqlite3_blob_open(
			conn.tls,
			conn.conn,
			cdb,
			ctable,
			ccolumn,
			row,
			writeFlag,
			blobPtrPtr,
		))
		switch res {
		case ResultLockedSharedCache:
			if err := reserr(waitForUnlockNotify(conn.tls, conn.conn, conn.unlockNote)); err != nil {
				return nil, fmt.Errorf("sqlite: open %q.%q blob: %w", table, column, err)
			}
			// loop
		case ResultOK:
			blobPtr := *(*uintptr)(unsafe.Pointer(blobPtrPtr))
			return &Blob{
				conn: conn,
				blob: blobPtr,
				size: lib.Xsqlite3_blob_bytes(conn.tls, blobPtr),
			}, nil
		default:
			return nil, fmt.Errorf("sqlite: open %q.%q blob: %w", table, column, conn.extreserr(res))
		}
	}
}

// Blob provides streaming access to SQLite blobs.
type Blob struct {
	conn *Conn
	blob uintptr
	off  int32
	size int32
}

// Read reads up to len(p) bytes from the blob into p.
// https://www.sqlite.org/c3ref/blob_read.html
func (blob *Blob) Read(p []byte) (n int, err error) {
	if blob.blob == 0 {
		return 0, fmt.Errorf("sqlite: read blob: %w", errInvalidBlob)
	}
	if blob.off >= blob.size {
		return 0, io.EOF
	}
	if err := blob.conn.interrupted(); err != nil {
		return 0, fmt.Errorf("sqlite: read blob: %w", err)
	}
	if rem := blob.size - blob.off; len(p) > int(rem) {
		p = p[:rem]
	}
	// TODO(someday): Avoid using actually unsafe pointer operation.
	res := ResultCode(lib.Xsqlite3_blob_read(blob.conn.tls, blob.blob, uintptr(unsafe.Pointer(&p[0])), int32(len(p)), blob.off))
	runtime.KeepAlive(p)
	if err := reserr(res); err != nil {
		return 0, fmt.Errorf("sqlite: read blob: %w", err)
	}
	blob.off += int32(len(p))
	return len(p), nil
}

// Write writes len(p) from p to the blob.
// https://www.sqlite.org/c3ref/blob_write.html
func (blob *Blob) Write(p []byte) (n int, err error) {
	if blob.blob == 0 {
		return 0, fmt.Errorf("sqlite: write blob: %w", errInvalidBlob)
	}
	if err := blob.conn.interrupted(); err != nil {
		return 0, fmt.Errorf("sqlite: write blob: %w", err)
	}
	// TODO(someday): Avoid using actually unsafe pointer operation.
	res := ResultCode(lib.Xsqlite3_blob_write(blob.conn.tls, blob.blob, uintptr(unsafe.Pointer(&p[0])), int32(len(p)), blob.off))
	runtime.KeepAlive(p)
	if err := reserr(res); err != nil {
		return 0, fmt.Errorf("sqlite: write blob: %w", err)
	}
	blob.off += int32(len(p))
	return len(p), nil
}

// Seek sets the offset for the next Read or Write and returns the offset.
// Seeking past the end of the blob returns an error.
func (blob *Blob) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		// use offset directly
	case io.SeekCurrent:
		offset += int64(blob.off)
	case io.SeekEnd:
		offset += int64(blob.size)
	default:
		return int64(blob.off), fmt.Errorf("sqlite: seek blob: invalid whence %d", whence)
	}
	if offset < 0 {
		return int64(blob.off), fmt.Errorf("sqlite: seek blob: negative offset %d", offset)
	}
	if offset > int64(blob.size) {
		return int64(blob.off), fmt.Errorf("sqlite: seek blob: offset %d is past size %d", offset, blob.size)
	}
	blob.off = int32(offset)
	return offset, nil
}

// Size returns the number of bytes in the blob.
func (blob *Blob) Size() int64 {
	return int64(blob.size)
}

// Close releases any resources associated with the blob handle.
// https://www.sqlite.org/c3ref/blob_close.html
func (blob *Blob) Close() error {
	if blob.blob == 0 {
		return errInvalidBlob
	}
	res := ResultCode(lib.Xsqlite3_blob_close(blob.conn.tls, blob.blob))
	blob.blob = 0
	if err := reserr(res); err != nil {
		return fmt.Errorf("sqlite: close blob: %w", err)
	}
	return nil
}

var errInvalidBlob = errors.New("invalid blob")

// TODO: Blob Reopen
