# Migrating from `crawshaw.io/sqlite`

`zombiezen.com/go/sqlite` is designed to mostly be a drop-in replacement for
`crawshaw.io/sqlite`. However, there are some incompatible API changes. To aid
in migrating, I've prepared a program that rewrites Go source code using
`crawshaw.io/sqlite` to use `zombiezen.com/go/sqlite`.

## Installation

```shell
go install zombiezen.com/go/sqlite/cmd/zombiezen-sqlite-migrate@latest
```

## Usage

Preview changes with:

```shell
zombiezen-sqlite-migrate ./...
```

And then apply them with:

```shell
zombiezen-sqlite-migrate -w ./...
```

## Automatically fixed changes

The `zombiezen-sqlite-migrate` tool automatically makes a number of mechanical
changes beyond changing the import paths to preserve semantics.

-  **`ErrorCode` renamed to `ResultCode`.** The `crawshaw.io/sqlite.ErrorCode` type
   actually represents a SQLite [result code][], not just error codes. To better
   capture this, the new type is named `zombiezen.com/go/sqlite.ResultCode`.
-  **Friendlier constant names.** The constant names in `crawshaw.io/sqlite`
   are written in upper snake case with `SQLITE_` prefixed (e.g.
   `sqlite.SQLITE_OK`); the constant names in `zombiezen.com/go/sqlite` are
   written in upper camel case with the type prefixed (e.g. `sqlite.ResultOK`).
-  `sqlitex.File` and `sqlitex.Buffer` are in `zombiezen.com/go/sqlite/sqlitefile`
   instead of `zombiezen.com/go/sqlite/sqlitex`.
-  The **session API** has some symbols renamed for clarity.
- `sqlitex.ExecFS` will rename to `sqlitex.ExecuteFS`,
  `sqlitex.ExecTransientFS` will rename to `sqlitex.ExecuteTransientFS`,
  and `sqlitex.ExecScriptFS` will rename to `sqlitex.ExecuteScriptFS`.

[result code]: https://sqlite.org/rescode.html

## Changes that require manual effort

Other usages of the `crawshaw.io/sqlite` may require manual effort to migrate,
but the `zombiezen-sqlite-migrate` tool will point them out.

### Application-Defined Functions

The `crawshaw.io/sqlite.Conn.CreateFunction` method and supporting APIs like
`Context` and `Value` have been re-tooled in `zombiezen.com/go/sqlite` with
better ergonomics. See the [`CreateFunction` reference][] for more details.

[`CreateFunction` reference]: https://pkg.go.dev/zombiezen.com/go/sqlite#Conn.CreateFunction

### Removed `Blob` methods

`zombiezen.com/go/sqlite.Blob` does not implement the [`io.ReaderAt`][] or
[`io.WriterAt`][] interfaces. Technically, neither did `crawshaw.io/sqlite.Blob`,
because it was not safe to call in parallel, which these interfaces require.
To avoid these methods being used incorrectly, I removed them.

I also removed the unused embedded interface fields.

[`io.ReaderAt`]: https://pkg.go.dev/io#ReaderAt
[`io.WriterAt`]: https://pkg.go.dev/io#WriterAt

### No dedicated `sqlite.Error` type

I don't want to commit to a specific error type in `zombiezen.com/go/sqlite`, so
there I removed the `Error` type. [`zombiezen.com/go/sqlite.ErrCode`][] still
extracts the `ResultCode` from an `error`, which covers most needs.

[`zombiezen.com/go/sqlite.ErrCode`]: https://pkg.go.dev/zombiezen.com/go/sqlite#ErrCode

### Authorizer `Action`

The [`zombiezen.com/go/sqlite.Action`][] uses accessor methods instead of struct
fields. Custom `Authorizer`s will need to be rewritten.

[`zombiezen.com/go/sqlite.Action`]: https://pkg.go.dev/zombiezen.com/go/sqlite#Action
