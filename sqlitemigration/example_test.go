// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlitemigration_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitemigration"
	"zombiezen.com/go/sqlite/sqlitex"
)

func Example() {
	schema := sqlitemigration.Schema{
		// Each element of the Migrations slice is applied in sequence. When you
		// want to change the schema, add a new SQL script to this list.
		//
		// Existing databases will pick up at the same position in the Migrations
		// slice as they last left off.
		Migrations: []string{
			"CREATE TABLE foo ( id INTEGER NOT NULL PRIMARY KEY );",

			"ALTER TABLE foo ADD COLUMN name TEXT;",
		},

		// The RepeatableMigration is run after all other Migrations if any
		// migration was run. It is useful for creating triggers and views.
		RepeatableMigration: "DROP VIEW IF EXISTS bar;\n" +
			"CREATE VIEW bar ( id, name ) AS SELECT id, name FROM foo;\n",
	}

	// Set up a temporary directory to store the database.
	dir, err := os.MkdirTemp("", "sqlitemigration")
	if err != nil {
		// handle error
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Open a pool. This does not block, and will start running any migrations
	// asynchronously.
	pool := sqlitemigration.NewPool(filepath.Join(dir, "foo.db"), schema, sqlitemigration.Options{
		Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		PrepareConn: func(conn *sqlite.Conn) error {
			// Enable foreign keys. See https://sqlite.org/foreignkeys.html
			return sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil)
		},
		OnError: func(e error) {
			log.Println(e)
		},
	})
	defer pool.Close()

	// Get a connection. This blocks until the migration completes.
	conn, err := pool.Get(context.TODO())
	if err != nil {
		// handle error
	}
	defer pool.Put(conn)

	// Print the list of schema objects created.
	const listSchemaQuery = `SELECT "type", "name" FROM sqlite_master ORDER BY 1, 2;`
	err = sqlitex.ExecuteTransient(conn, listSchemaQuery, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Printf("%-5s %s\n", stmt.ColumnText(0), stmt.ColumnText(1))
			return nil
		},
	})
	if err != nil {
		// handle error
	}

	// Output:
	// table foo
	// view  bar
}

// This example constructs a schema from a set of SQL files in a directory named
// schema01.sql, schema02.sql, etc.
func ExampleSchema() {
	var schema sqlitemigration.Schema
	for i := 1; ; i++ {
		migration, err := os.ReadFile(fmt.Sprintf("schema%02d.sql", i))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			// handle error...
		}
		schema.Migrations = append(schema.Migrations, string(migration))
	}
}
