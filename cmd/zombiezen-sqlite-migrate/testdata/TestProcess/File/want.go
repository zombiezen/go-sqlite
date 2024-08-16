// Copyright 2021 Roxy Light
// SPDX-License-Identifier: ISC

package main

import (
	"zombiezen.com/go/sqlite/sqlitefile"
)

func main() {
	var f *sqlitefile.File
	f, _ = sqlitefile.NewFile(nil)
	var buf *sqlitefile.Buffer
	buf, _ = sqlitefile.NewBuffer(nil)

	_, _ = f, buf
}
