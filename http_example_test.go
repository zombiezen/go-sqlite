// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"fmt"
	"log"
	"net/http"

	"zombiezen.com/go/sqlite/sqlitex"
)

var dbpool *sqlitex.Pool

// Using a Pool to execute SQL in a concurrent HTTP handler.
func Example_http() {
	var err error
	dbpool, err = sqlitex.Open("file:memory:?mode=memory", 0, 10)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", handle)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handle(w http.ResponseWriter, r *http.Request) {
	conn := dbpool.Get(r.Context())
	if conn == nil {
		return
	}
	defer dbpool.Put(conn)
	stmt := conn.Prep("SELECT foo FROM footable WHERE id = $id;")
	stmt.SetText("$id", "_user_id_")
	for {
		if hasRow, err := stmt.Step(); err != nil {
			// ... handle error
		} else if !hasRow {
			break
		}
		foo := stmt.GetText("foo")
		// ... use foo
		fmt.Fprintln(w, foo)
	}
}
