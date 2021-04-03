// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package main

import "crawshaw.io/sqlite/sqlitex"

func main() {
	var f *sqlitex.File
	f, _ = sqlitex.NewFile(nil)
	var buf *sqlitex.Buffer
	buf, _ = sqlitex.NewBuffer(nil)

	_, _ = f, buf
}
