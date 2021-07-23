# `zombiezen.com/go/sqlite` Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

[Unreleased]: https://github.com/zombiezen/go-sqlite/compare/v0.5.0...main

## [Unreleased][]

### Removed

- Removed `OpenFlags` that are only used for VFS.

### Fixed

- Properly clean up WAL when using `sqlitex.Pool`
  ([#14](https://github.com/zombiezen/go-sqlite/issues/14))

## [0.5.0][] - 2021-05-22

[0.5.0]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.5.0

### Added

- Added `shell` package with basic [REPL][]
- Added `SetAuthorizer`, `Limit`, and `SetDefensive` methods to `*Conn` for use
  in ([#12](https://github.com/zombiezen/go-sqlite/issues/12))
- Added `Version` and `VersionNumber` constants

[REPL]: https://en.wikipedia.org/wiki/Read%E2%80%93eval%E2%80%93print_loop

### Fixed

- Documented compiled-in extensions ([#11](https://github.com/zombiezen/go-sqlite/issues/11))
- Internal objects are no longer susceptible to ID wraparound issues
  ([#13](https://github.com/zombiezen/go-sqlite/issues/13))

## [0.4.0][] - 2021-05-13

[0.4.0]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.4.0

### Added

- Add Context.Conn method ([#10](https://github.com/zombiezen/go-sqlite/issues/10))
- Add methods to get and set auxiliary function data
  ([#3](https://github.com/zombiezen/go-sqlite/issues/3))

## [0.3.1][] - 2021-05-03

[0.3.1]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.3.1

### Fixed

- Fix conversion of BLOB to TEXT when returning BLOB from a user-defined function

## [0.3.0][] - 2021-04-27

[0.3.0]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.3.0

### Added

- Implement `io.StringWriter`, `io.ReaderFrom`, and `io.WriterTo` on `Blob`
  ([#2](https://github.com/zombiezen/go-sqlite/issues/2))
- Add godoc examples for `Blob`, `sqlitemigration`, and `SetInterrupt`
- Add more README documentation

## [0.2.2][] - 2021-04-24

[0.2.2]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.2.2

### Changed

- Simplified license to [ISC](https://github.com/zombiezen/go-sqlite/blob/v0.2.2/LICENSE)

### Fixed

- Updated version of `modernc.org/sqlite` to 1.10.4 to use [mutex initialization](https://gitlab.com/cznic/sqlite/-/issues/52)
- Fixed doc comment for `BindZeroBlob`

## [0.2.1][] - 2021-04-17

[0.2.1]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.2.1

### Fixed

- Removed bogus import comment

## [0.2.0][] - 2021-04-03

[0.2.0]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.2.0

### Added

- New migration tool. See [the README](https://github.com/zombiezen/go-sqlite/blob/v0.2.0/cmd/zombiezen-sqlite-migrate/README.md)
  to get started. ([#1](https://github.com/zombiezen/go-sqlite/issues/1))

### Changed

- `*Conn.CreateFunction` has changed entirely. See the
  [reference](https://pkg.go.dev/zombiezen.com/go/sqlite#Conn.CreateFunction)
  for details.
- `sqlitex.File` and `sqlitex.Buffer` have been moved to the `sqlitefile` package
- The `sqlitefile.Exec*` functions have been moved to the `sqlitex` package
  as `Exec*FS`.

## [0.1.0][] - 2021-03-31

Initial release

[0.1.0]: https://github.com/zombiezen/go-sqlite/releases/tag/v0.1.0
