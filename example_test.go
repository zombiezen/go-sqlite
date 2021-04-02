// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"fmt"
	"regexp"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
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

func ExampleConn_CreateFunction() {
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()

	// Add a regexp(pattern, string) function.
	err = conn.CreateFunction("regexp", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
			re, err := regexp.Compile(args[0].Text())
			if err != nil {
				return sqlite.Value{}, fmt.Errorf("regexp: %w", err)
			}
			found := 0
			if re.MatchString(args[1].Text()) {
				found = 1
			}
			return sqlite.IntegerValue(int64(found)), nil
		},
	})
	if err != nil {
		// handle error
	}

	matches, err := sqlitex.ResultBool(conn.Prep(`SELECT regexp('fo+', 'foo');`))
	if err != nil {
		// handle error
	}
	fmt.Println("First matches:", matches)

	matches, err = sqlitex.ResultBool(conn.Prep(`SELECT regexp('fo+', 'bar');`))
	if err != nil {
		// handle error
	}
	fmt.Println("Second matches:", matches)

	// Output:
	// First matches: true
	// Second matches: false
}
