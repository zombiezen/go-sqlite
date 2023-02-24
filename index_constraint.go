// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

import (
	"fmt"

	lib "modernc.org/sqlite/lib"
)

// IndexConstraint is a constraint term in the WHERE clause
// of a query that uses a virtual table.
type IndexConstraint struct {
	// Column is the left-hand operand.
	// Column indices start at 0.
	// -1 indicates the left-hand operand is the rowid.
	// Column should be ignored when Op is [IndexConstraintLimit] or [IndexConstraintOffset].
	Column int
	// Op is the constraint's operator.
	Op IndexConstraintOp
	// Usable indicates whether [VTable.BestIndex] should consider the constraint.
	// Usable may false depending on how tables are ordered in a join.
	Usable bool
	// Collation is the name of the collating sequence
	// that should be used when evaluating the constraint.
	Collation string
	// RValue is the right-hand operand, if known during statement preparation.
	// It's only valid until the end of [VTable.BestIndex].
	RValue Value
	// RValueKnown indicates whether RValue is set.
	RValueKnown bool
}

// IndexConstraintOp is an enumeration of virtual table constraint operators
// used in [IndexConstraint].
type IndexConstraintOp uint8

const (
	IndexConstraintEq        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_EQ
	IndexConstraintGT        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_GT
	IndexConstraintLE        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_LE
	IndexConstraintLT        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_LT
	IndexConstraintGE        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_GE
	IndexConstraintMatch     IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_MATCH
	IndexConstraintLike      IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_LIKE
	IndexConstraintGlob      IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_GLOB
	IndexConstraintRegexp    IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_REGEXP
	IndexConstraintNE        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_NE
	IndexConstraintIsNot     IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_ISNOT
	IndexConstraintIsNotNull IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_ISNOTNULL
	IndexConstraintIsNull    IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_ISNULL
	IndexConstraintIs        IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_IS
	IndexConstraintLimit     IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_LIMIT
	IndexConstraintOffset    IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_OFFSET
)

const indexConstraintFunction IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_FUNCTION

// String returns the operator symbol or keyword.
func (op IndexConstraintOp) String() string {
	switch op {
	case IndexConstraintEq:
		return "="
	case IndexConstraintGT:
		return ">"
	case IndexConstraintLE:
		return "<="
	case IndexConstraintLT:
		return "<"
	case IndexConstraintGE:
		return ">="
	case IndexConstraintMatch:
		return "MATCH"
	case IndexConstraintLike:
		return "LIKE"
	case IndexConstraintGlob:
		return "GLOB"
	case IndexConstraintRegexp:
		return "REGEXP"
	case IndexConstraintNE:
		return "<>"
	case IndexConstraintIsNot:
		return "IS NOT"
	case IndexConstraintIsNotNull:
		return "IS NOT NULL"
	case IndexConstraintIsNull:
		return "IS NULL"
	case IndexConstraintIs:
		return "IS"
	case IndexConstraintLimit:
		return "LIMIT"
	case IndexConstraintOffset:
		return "OFFSET"
	default:
		if op < indexConstraintFunction {
			return fmt.Sprintf("IndexConstraintOp(%d)", uint8(op))
		}
		return fmt.Sprintf("<function %d>", uint8(op))
	}
}
