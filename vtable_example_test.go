// Copyright 2023 Roxy Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"fmt"
	"log"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func ExampleVTable() {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	err = conn.SetModule("templatevtab", &sqlite.Module{
		Connect: templatevtabConnect,
	})
	if err != nil {
		log.Fatal(err)
	}
	err = sqlitex.ExecuteTransient(
		conn,
		`SELECT a, b FROM templatevtab ORDER BY rowid;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				fmt.Printf("%4d, %4d\n", stmt.ColumnInt(0), stmt.ColumnInt(1))
				return nil
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	// 1001, 2001
	// 1002, 2002
	// 1003, 2003
	// 1004, 2004
	// 1005, 2005
	// 1006, 2006
	// 1007, 2007
	// 1008, 2008
	// 1009, 2009
	// 1010, 2010
}

type templatevtab struct{}

const (
	templatevarColumnA = iota
	templatevarColumnB
)

func templatevtabConnect(c *sqlite.Conn, opts *sqlite.VTableConnectOptions) (sqlite.VTable, *sqlite.VTableConfig, error) {
	vtab := new(templatevtab)
	cfg := &sqlite.VTableConfig{
		Declaration: "CREATE TABLE x(a,b)",
	}
	return vtab, cfg, nil
}

func (vt *templatevtab) BestIndex(*sqlite.IndexInputs) (*sqlite.IndexOutputs, error) {
	return &sqlite.IndexOutputs{
		EstimatedCost: 10,
		EstimatedRows: 10,
	}, nil
}

func (vt *templatevtab) Open() (sqlite.VTableCursor, error) {
	return &templatevtabCursor{rowid: 1}, nil
}

func (vt *templatevtab) Disconnect() error {
	return nil
}

func (vt *templatevtab) Destroy() error {
	return nil
}

type templatevtabCursor struct {
	rowid int64
}

func (cur *templatevtabCursor) Filter(id sqlite.IndexID, argv []sqlite.Value) error {
	cur.rowid = 1
	return nil
}

func (cur *templatevtabCursor) Next() error {
	cur.rowid++
	return nil
}

func (cur *templatevtabCursor) Column(i int, noChange bool) (sqlite.Value, error) {
	switch i {
	case templatevarColumnA:
		return sqlite.IntegerValue(1000 + cur.rowid), nil
	case templatevarColumnB:
		return sqlite.IntegerValue(2000 + cur.rowid), nil
	default:
		panic("unreachable")
	}
}

func (cur *templatevtabCursor) RowID() (int64, error) {
	return cur.rowid, nil
}

func (cur *templatevtabCursor) EOF() bool {
	return cur.rowid > 10
}

func (cur *templatevtabCursor) Close() error {
	return nil
}
