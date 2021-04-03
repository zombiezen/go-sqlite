// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Test stubs for crawshaw.io/sqlite/sqlitex.
package sqlitex

import "crawshaw.io/sqlite"

type File struct {
	io.Reader
	io.Writer
	io.Seeker
}

func NewFile(conn *sqlite.Conn) (*File, error) { return nil, nil }

func NewFileSize(conn *sqlite.Conn, initSize int) (*File, error) { return nil, nil }

type Buffer struct {
	io.Reader
	io.Writer
	io.ByteScanner
}

func NewBuffer(conn *sqlite.Conn) (*Buffer, error) { return nil, nil }

func NewBufferSize(conn *sqlite.Conn, pageSize int) (*Buffer, error) { return nil, nil }
