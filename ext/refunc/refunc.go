// Copyright 2023 Roxy Light
// SPDX-License-Identifier: ISC

// Package refunc provides an implementation of the [REGEXP operator]
// that uses the Go [regexp] package.
//
// [REGEXP operator]: https://sqlite.org/lang_expr.html#the_like_glob_regexp_match_and_extract_operators
package refunc

import (
	"fmt"
	"regexp"

	"zombiezen.com/go/sqlite"
)

// Impl is the implementation of the REGEXP function.
var Impl = &sqlite.FunctionImpl{
	NArgs:         2,
	Deterministic: true,
	AllowIndirect: true,
	Scalar:        regexpFunc,
}

// Register registers the "regexp" function on the given connection.
func Register(c *sqlite.Conn) error {
	return c.CreateFunction("regexp", Impl)
}

func regexpFunc(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
	// First: attempt to retrieve the compiled regexp from a previous call.
	re, ok := ctx.AuxData(0).(*regexp.Regexp)
	if !ok {
		// Auxiliary data not present. Either this is the first call with this
		// argument, or SQLite has discarded the auxiliary data.
		var err error
		re, err = regexp.Compile(args[0].Text())
		if err != nil {
			return sqlite.Value{}, fmt.Errorf("regexp: %w", err)
		}
		// Store the auxiliary data for future calls.
		ctx.SetAuxData(0, re)
	}

	found := 0
	if re.MatchString(args[1].Text()) {
		found = 1
	}
	return sqlite.IntegerValue(int64(found)), nil
}
