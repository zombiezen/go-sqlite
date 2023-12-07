// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package refunc

import (
	"testing"

	"zombiezen.com/go/sqlite"
)

func TestImpl(t *testing.T) {
	c, err := sqlite.OpenConn("", sqlite.OpenMemory|sqlite.OpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	if err := Register(c); err != nil {
		t.Error("Register:", err)
	}

	tests := []struct {
		x, y string
		want bool
	}{
		{"", "foo", false},
		{"foo", "", true},
		{"foo", "^fo*$", true},
		{"bar", "^fo*$", false},
	}
	stmt := c.Prep("VALUES (:x REGEXP :y);")
	for _, test := range tests {
		stmt.SetText(":x", test.x)
		stmt.SetText(":y", test.y)
		rowReturned, err := stmt.Step()
		if err != nil {
			t.Errorf("%q REGEXP %q: %v", test.x, test.y, err)
			stmt.Reset()
			continue
		}
		if !rowReturned {
			t.Errorf("%q REGEXP %q: no row returned", test.x, test.y)
			stmt.Reset()
			continue
		}
		if got := stmt.ColumnBool(0); got != test.want {
			t.Errorf("%q REGEXP %q = %t; want %t", test.x, test.y, got, test.want)
		}
		if err := stmt.Reset(); err != nil {
			t.Errorf("%q REGEXP %q: %v", test.x, test.y, err)
		}
	}
}
