// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"fmt"
	"log"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// This is a port of the [generate_series] table-valued function in the SQLite tree.
//
// [generate_series]: https://sqlite.org/src/file/ext/misc/series.c
func ExampleVTable_series() {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	err = conn.SetModule("generate_series", &sqlite.Module{
		Connect: seriesvtabConnect,
	})
	if err != nil {
		log.Fatal(err)
	}
	err = sqlitex.ExecuteTransient(
		conn,
		`SELECT * FROM generate_series(0, 20, 5);`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				fmt.Printf("%2d\n", stmt.ColumnInt(0))
				return nil
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	//  0
	//  5
	// 10
	// 15
	// 20
}

type seriesvtab struct{}

const (
	seriesColumnValue = iota
	seriesColumnStart
	seriesColumnStop
	seriesColumnStep
)

func seriesvtabConnect(c *sqlite.Conn, opts *sqlite.VTableConnectOptions) (sqlite.VTable, *sqlite.VTableConfig, error) {
	vtab := new(seriesvtab)
	cfg := &sqlite.VTableConfig{
		Declaration:   "CREATE TABLE x(value,start hidden,stop hidden,step hidden)",
		AllowIndirect: true,
	}
	return vtab, cfg, nil
}

// BestIndex looks for equality constraints against the hidden start, stop, and step columns,
// and if present, it uses those constraints to bound the sequence of generated values.
// If the equality constraints are missing, it uses 0 for start, 4294967295 for stop,
// and 1 for step.
// BestIndex returns a small cost when both start and stop are available,
// and a very large cost if either start or stop are unavailable.
// This encourages the query planner to order joins such that the bounds of the
// series are well-defined.
//
// SQLite will invoke this method one or more times
// while planning a query that uses the generate_series virtual table.
// This routine needs to create a query plan for each invocation
// and compute an estimated cost for that plan.
//
// In this implementation ID.Num is used to represent the query plan.
// ID.String is unused.
//
// The query plan is represented by bits in idxNum:
//
//	(1)  start = $value  -- constraint exists
//	(2)  stop = $value   -- constraint exists
//	(4)  step = $value   -- constraint exists
//	(8)  output in descending order
func (vt *seriesvtab) BestIndex(inputs *sqlite.IndexInputs) (*sqlite.IndexOutputs, error) {
	var idxNum int32
	startSeen := false
	var unusableMask uint
	aIdx := [3]int{-1, -1, -1}
	for i, c := range inputs.Constraints {
		if c.Column < seriesColumnStart {
			continue
		}
		col := c.Column - seriesColumnStart // [0, 2]
		mask := uint(1 << col)
		if col == 0 {
			startSeen = true
		}
		if !c.Usable {
			unusableMask |= mask
			continue
		}
		if c.Op == sqlite.IndexConstraintEq {
			idxNum |= int32(mask)
			aIdx[col] = i
		}
	}
	outputs := &sqlite.IndexOutputs{
		ID:              sqlite.IndexID{Num: idxNum},
		ConstraintUsage: make([]sqlite.IndexConstraintUsage, len(inputs.Constraints)),
	}
	nArg := 0
	for _, j := range aIdx {
		if j >= 0 {
			nArg++
			outputs.ConstraintUsage[j] = sqlite.IndexConstraintUsage{
				ArgvIndex: nArg,
				Omit:      true,
			}
		}
	}
	if !startSeen {
		return nil, fmt.Errorf("first argument to \"generate_series()\" missing or unusable")
	}
	if unusableMask&^uint(idxNum) != 0 {
		// The start, stop, and step columns are inputs.
		// Therefore if there are unusable constraints on any of start, stop, or step then
		// this plan is unusable.
		return nil, sqlite.ResultConstraint.ToError()
	}
	if idxNum&3 == 3 {
		// Both start= and stop= boundaries are available.
		// This is the preferred case.
		if idxNum&4 != 0 {
			outputs.EstimatedCost = 1
		} else {
			outputs.EstimatedCost = 2
		}
		outputs.EstimatedRows = 1000
		if len(inputs.OrderBy) >= 1 && inputs.OrderBy[0].Column == 0 {
			if inputs.OrderBy[0].Desc {
				idxNum |= 8
			} else {
				idxNum |= 16
			}
			outputs.OrderByConsumed = true
		}
	} else {
		// If either boundary is missing, we have to generate a huge span of numbers.
		// Make this case very expensive so that the query planner will work hard to avoid it.
		outputs.EstimatedRows = 2147483647
	}
	return outputs, nil
}

func (vt *seriesvtab) Open() (sqlite.VTableCursor, error) {
	return new(seriesvtabCursor), nil
}

func (vt *seriesvtab) Disconnect() error {
	return nil
}

func (vt *seriesvtab) Destroy() error {
	return nil
}

type seriesvtabCursor struct {
	isDesc  bool
	rowid   int64
	value   int64
	mnValue int64
	mxValue int64
	step    int64
}

// Filter is called to "rewind" the cursor object back to the first row of output.
// This method is always called at least once
// prior to any call to Column or RowID or EOF.
//
// The query plan selected by BestIndex is passed in the id parameter.
// (id.String is not used in this implementation.)
// id.Num is a bitmask showing which constraints are available:
//
//	1: start=VALUE
//	2: stop=VALUE
//	4: step=VALUE
//
// Also, if bit 8 is set, that means that the series should be output in descending order
// rather than in ascending order.
// If bit 16 is set, then output must appear in ascending order.
//
// This routine should initialize the cursor and position it
// so that it is pointing at the first row,
// or pointing off the end of the table (so that EOF will return true)
// if the table is empty.
func (cur *seriesvtabCursor) Filter(id sqlite.IndexID, argv []sqlite.Value) error {
	i := 0
	if id.Num&1 != 0 {
		cur.mnValue = argv[i].Int64()
		i++
	} else {
		cur.mnValue = 0
	}
	if id.Num&2 != 0 {
		cur.mxValue = argv[i].Int64()
		i++
	} else {
		cur.mxValue = 0xffffffff
	}
	if id.Num&4 != 0 {
		cur.step = argv[i].Int64()
		i++
		if cur.step == 0 {
			cur.step = 1
		} else if cur.step < 0 {
			cur.step = -cur.step
			if id.Num&16 == 0 {
				id.Num |= 8
			}
		}
	} else {
		cur.step = 1
	}
	for _, arg := range argv {
		if arg.Type() == sqlite.TypeNull {
			// If any of the constraints have a NULL value, then return no rows.
			// See ticket https://www.sqlite.org/src/info/fac496b61722daf2
			cur.mnValue = 1
			cur.mxValue = 0
			break
		}
	}
	if id.Num&8 != 0 {
		cur.isDesc = true
		cur.value = cur.mxValue
		if cur.step > 0 {
			cur.value -= (cur.mxValue - cur.mnValue) % cur.step
		}
	} else {
		cur.isDesc = false
		cur.value = cur.mnValue
	}
	cur.rowid = 1
	return nil
}

func (cur *seriesvtabCursor) Next() error {
	if cur.isDesc {
		cur.value -= cur.step
	} else {
		cur.value += cur.step
	}
	cur.rowid++
	return nil
}

func (cur *seriesvtabCursor) Column(i int) (sqlite.Value, error) {
	switch i {
	case seriesColumnValue:
		return sqlite.IntegerValue(cur.value), nil
	case seriesColumnStart:
		return sqlite.IntegerValue(cur.mnValue), nil
	case seriesColumnStop:
		return sqlite.IntegerValue(cur.mxValue), nil
	case seriesColumnStep:
		return sqlite.IntegerValue(cur.step), nil
	default:
		panic("unreachable")
	}
}

func (cur *seriesvtabCursor) RowID() (int64, error) {
	return cur.rowid, nil
}

func (cur *seriesvtabCursor) EOF() bool {
	if cur.isDesc {
		return cur.value < cur.mnValue
	} else {
		return cur.value > cur.mxValue
	}
}

func (cur *seriesvtabCursor) Close() error {
	return nil
}
