// Copyright 2021 Roxy Light
// SPDX-License-Identifier: ISC

package main

import (
	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"zombiezen.com/go/bass/sql/sqlitefile"
)

func main() {
	var conn *sqlite.Conn
	var file *sqlitex.File
	sqlitefile.ExecScript(conn, nil, "foo.sql", &sqlitefile.ExecOptions{
		Args: []interface{}{1, "foo"},
	})
	sqlitex.Exec(conn, `SELECT 1;`, nil)
	_ = file
}
