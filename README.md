# `zombiezen.com/go/sqlite`

This package provides a low-level Go interface to SQLite 3. It is a fork of
[`crawshaw.io/sqlite`][] that uses [`modernc.org/sqlite`][]. It aims to provide
mostly the same API as `crawshaw.io/sqlite`, with some fixes.

[`crawshaw.io/sqlite`]: https://github.com/crawshaw/sqlite
[`modernc.org/sqlite`]: https://pkg.go.dev/modernc.org/sqlite

## License

Mostly ISC, with some code borrowed from `modernc.org/sqlite`, which is under a
BSD 3-Clause license. See [LICENSE](LICENSE) for details.

Source files in this repository use [SPDX-License-Identifier tags][] to indicate
the applicable license.

[SPDX-License-Identifier tags]: https://spdx.github.io/spdx-spec/appendix-V-using-SPDX-short-identifiers-in-source-files/
