// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Test stubs for zombiezen.com/go/sqlite/sqlitex.
package sqlitex

import (
	"os"

	"zombiezen.com/go/sqlite"
)

type ExecOptions struct {
	Args       []interface{}
	Named      map[string]interface{}
	ResultFunc func(stmt *sqlite.Stmt) error
}

func ExecScriptFS(conn *sqlite.Conn, fsys FS, filename string, opts *ExecOptions) error {
	return nil
}

func ExecuteScriptFS(conn *sqlite.Conn, fsys FS, filename string, opts *ExecOptions) error {
	return nil
}

// FS is a copy of Go 1.16's io/fs.FS interface.
type FS interface {
	Open(name string) (File, error)
}

// File is a copy of Go 1.16's io/fs.File interface.
type File interface {
	Stat() (os.FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}
