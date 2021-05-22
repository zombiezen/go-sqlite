// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"context"
	"fmt"
	"regexp"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func Example() {
	// Open an in-memory database.
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()

	// Execute a query.
	err = sqlitex.ExecTransient(conn, "SELECT 'hello, world';", func(stmt *sqlite.Stmt) error {
		fmt.Println(stmt.ColumnText(0))
		return nil
	})
	if err != nil {
		// handle error
	}

	// Output:
	// hello, world
}

// This is the same as the main package example, but uses the SQLite
// statement API instead of sqlitex.
func Example_withoutX() {
	// Open an in-memory database.
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()

	// Prepare a statement.
	stmt, _, err := conn.PrepareTransient("SELECT 'hello, world';")
	if err != nil {
		// handle error
	}
	// Transient statements must always be finalized.
	defer stmt.Finalize()

	for {
		row, err := stmt.Step()
		if err != nil {
			// handle error
		}
		if !row {
			break
		}
		fmt.Println(stmt.ColumnText(0))
	}

	// Output:
	// hello, world
}

func ExampleConn_SetInterrupt() {
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// You can use the Done() channel from a context to set deadlines and timeouts
	// on queries.
	ctx := context.TODO()
	conn.SetInterrupt(ctx.Done())
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

// This example shows the same regexp function as in the CreateFunction example,
// but it uses auxiliary data to avoid recompiling the regular expression.
func ExampleContext_AuxData() {
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()

	// Add a regexp(pattern, string) function.
	usedAux := false
	err = conn.CreateFunction("regexp", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
			// First: attempt to retrieve the compiled regexp from a previous call.
			re, ok := ctx.AuxData(0).(*regexp.Regexp)
			if ok {
				usedAux = true
			} else {
				// Auxiliary data not present. Either this is the first call with this
				// argument, or SQLite has discarded the auxiliary data.
				var err error
				re, err = regexp.Compile(args[0].Text())
				if err != nil {
					return sqlite.Value{}, fmt.Errorf("regexp: %w", err)
				}
				// Store the auxiliary data for future calls.
				ctx.SetAuxData(0, re)
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

	const query = `WITH test_strings(i, s) AS (VALUES (1, 'foo'), (2, 'bar')) ` +
		`SELECT i, regexp('fo+', s) FROM test_strings ORDER BY i;`
	err = sqlitex.ExecTransient(conn, query, func(stmt *sqlite.Stmt) error {
		fmt.Printf("Match %d: %t\n", stmt.ColumnInt(0), stmt.ColumnInt(1) != 0)
		return nil
	})
	if err != nil {
		// handle error
	}
	if usedAux {
		fmt.Println("Used aux data to speed up query!")
	}
	// Output:
	// Match 1: true
	// Match 2: false
	// Used aux data to speed up query!
}

func ExampleBlob() {
	// Create a new database with a "blobs" table with a single column, "myblob".
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()
	err = sqlitex.ExecTransient(conn, `CREATE TABLE blobs (myblob blob);`, nil)
	if err != nil {
		// handle error
	}

	// Insert a new row with enough space for the data we want to insert.
	const dataToInsert = "Hello, World!"
	err = sqlitex.ExecTransient(
		conn,
		`INSERT INTO blobs (myblob) VALUES (zeroblob(?));`,
		nil,
		len(dataToInsert),
	)
	if err != nil {
		// handle error
	}

	// Open a handle to the "myblob" column on the row we just inserted.
	blob, err := conn.OpenBlob("", "blobs", "myblob", conn.LastInsertRowID(), true)
	if err != nil {
		// handle error
	}
	_, writeErr := blob.WriteString(dataToInsert)
	closeErr := blob.Close()
	if writeErr != nil {
		// handle error
	}
	if closeErr != nil {
		// handle error
	}

	// Read back the blob.
	var data []byte
	err = sqlitex.ExecTransient(conn, `SELECT myblob FROM blobs;`, func(stmt *sqlite.Stmt) error {
		data = make([]byte, stmt.ColumnLen(0))
		stmt.ColumnBytes(0, data)
		return nil
	})
	if err != nil {
		// handle error
	}
	fmt.Printf("%s\n", data)

	// Output:
	// Hello, World!
}

func ExampleConn_SetAuthorizer() {
	// Create a new database.
	conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
	if err != nil {
		// handle error
	}
	defer conn.Close()

	// Set an authorizer that prevents any mutations.
	err = conn.SetAuthorizer(sqlite.AuthorizeFunc(func(action sqlite.Action) sqlite.AuthResult {
		typ := action.Type()
		if typ == sqlite.OpSelect ||
			typ == sqlite.OpRead ||
			// Permit function calls.
			typ == sqlite.OpFunction ||
			// Permit transactions.
			typ == sqlite.OpTransaction ||
			typ == sqlite.OpSavepoint {
			return sqlite.AuthResultOK
		}
		return sqlite.AuthResultDeny
	}))
	if err != nil {
		// handle error
	}

	// Authorizers operate during statement preparation, so this will succeed:
	stmt, _, err := conn.PrepareTransient(`SELECT 'Hello, World!';`)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("Read-only statement prepared!")
		if err := stmt.Finalize(); err != nil {
			panic(err)
		}
	}

	// But this will not:
	stmt, _, err = conn.PrepareTransient(`CREATE TABLE foo (id INTEGER PRIMARY KEY);`)
	if err != nil {
		fmt.Println("Prepare CREATE TABLE failed with code", sqlite.ErrCode(err))
	} else if err := stmt.Finalize(); err != nil {
		panic(err)
	}
	// Output:
	// Read-only statement prepared!
	// Prepare CREATE TABLE failed with code SQLITE_AUTH
}
