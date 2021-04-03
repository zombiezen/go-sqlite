// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package main

import (
	"zombiezen.com/go/sqlite"
	sqlitefile2 "zombiezen.com/go/sqlite/sqlitefile"
	"zombiezen.com/go/sqlite/sqlitex"
)

func main() {
	var conn *sqlite.Conn
	var file *sqlitefile2.File
	sqlitex.ExecScriptFS(conn, nil, "foo.sql", &sqlitex.ExecOptions{
		Args: []interface{}{1, "foo"},
	})
	sqlitex.Exec(conn, `SELECT 1;`, nil)
	_ = file
}
