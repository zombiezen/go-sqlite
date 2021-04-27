# `zombiezen.com/go/sqlite`

[![Go Reference](https://pkg.go.dev/badge/zombiezen.com/go/sqlite.svg)][reference docs]

This package provides a low-level Go interface to [SQLite 3][]. It is a fork of
[`crawshaw.io/sqlite`][] that uses [`modernc.org/sqlite`][], a CGo-free SQLite
package.  It aims to be a mostly drop-in replacement for
`crawshaw.io/sqlite`.  See the [migration docs][] for instructions on how to
migrate.

[`crawshaw.io/sqlite`]: https://github.com/crawshaw.io/sqlite
[`modernc.org/sqlite`]: https://pkg.go.dev/modernc.org/sqlite
[migration docs]: cmd/zombiezen-sqlite-migrate/README.md
[reference docs]: https://pkg.go.dev/zombiezen.com/go/sqlite
[SQLite 3]: https://sqlite.org/

## Install

```shell
go get zombiezen.com/go/sqlite
```

## Getting Started

```go
import (
  "fmt"

  "zombiezen.com/go/sqlite"
  "zombiezen.com/go/sqlite/sqlitex"
)

// ...

// Open an in-memory database.
conn, err := sqlite.OpenConn(":memory:", sqlite.OpenReadWrite)
if err != nil {
  return err
}
defer conn.Close()

// Execute a query.
err = sqlitex.ExecTransient(conn, "SELECT 'hello, world';", func(stmt *sqlite.Stmt) error {
  fmt.Println(stmt.ColumnText(0))
  return nil
})
if err != nil {
  return err
}
```

If you're creating a new application, see the [package examples][] or the
[reference docs][].

If you're looking to switch existing code that uses `crawshaw.io/sqlite`, take
a look at the [migration docs][].

[package examples]: https://pkg.go.dev/zombiezen.com/go/sqlite#pkg-examples

## License

[ISC](LICENSE)
