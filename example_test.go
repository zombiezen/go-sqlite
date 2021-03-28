// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"fmt"

	"zombiezen.com/go/sqlite"
)

func Example() {
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	stmt, _, err := conn.PrepareTransient("SELECT 'hello, world';")
	if err != nil {
		panic(err)
	}
	defer stmt.Finalize()
	for {
		row, err := stmt.Step()
		if err != nil {
			panic(err)
		}
		if !row {
			return
		}
		fmt.Println(stmt.ColumnText(0))
	}

	// Output:
	// hello, world
}
