// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Test stubs for crawshaw.io/sqlite.
package sqlite

type Conn struct {
}

func OpenConn(path string, flags OpenFlags) (*Conn, error) { return nil, nil }

func (c *Conn) GetAutocommit() bool { return false }

type ErrorCode int

const SQLITE_OK = ErrorCode(0)
