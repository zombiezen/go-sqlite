# Go interface to SQLite.

[![GoDoc](https://godoc.org/crawshaw.io/sqlite?status.svg)](https://godoc.org/crawshaw.io/sqlite) [![Build Status](https://travis-ci.org/crawshaw/sqlite.svg?branch=master)](https://travis-ci.org/crawshaw/sqlite) (linux and macOS) [![Build status](https://ci.appveyor.com/api/projects/status/jh9xx6cut73ufkl8?svg=true)](https://ci.appveyor.com/project/crawshaw/sqlite) (windows)

This package provides a low-level Go interface to SQLite 3. Connections are [pooled](https://godoc.org/crawshaw.io/sqlite#Pool) and if the SQLite [shared cache](https://www.sqlite.org/sharedcache.html) mode is enabled the package takes advantage of the [unlock-notify API](https://www.sqlite.org/unlock_notify.html) to minimize the amount of handling user code needs for dealing with database lock contention.

It has interfaces for some of SQLite's more interesting extensions, such as [incremental BLOB I/O](https://www.sqlite.org/c3ref/blob_open.html) and the [session extension](https://www.sqlite.org/sessionintro.html).

A utility package, [sqlitex](https://godoc.org/crawshaw.io/sqlite/sqlitex), provides some higher-level tools for making it easier to perform common tasks with SQLite. In particular it provides support to make nested transactions easy to use via [sqlitex.Save](https://godoc.org/crawshaw.io/sqlite/sqlitex#Save).

This is not a database/sql driver.

```go get -u crawshaw.io/sqlite```

## Example

A HTTP handler that uses a multi-threaded pool of SQLite connections via a shared cache.

```go
var dbpool *sqlitex.Pool

func main() {
	var err error
	dbpool, err = sqlitex.Open("file:memory:?mode=memory", 0, 10)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
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
	}
}
```

https://godoc.org/crawshaw.io/sqlite
