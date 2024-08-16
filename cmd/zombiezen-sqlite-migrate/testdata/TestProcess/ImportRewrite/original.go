// Copyright 2021 Roxy Light
// SPDX-License-Identifier: ISC

package main

import "crawshaw.io/sqlite"

func main() {
	var db *sqlite.Conn
	var err error
	db, err = sqlite.OpenConn(":memory:", 0)
	_, _ = db, err
}
