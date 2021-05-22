// Copyright (c) 2018 David Crawshaw <david@zentus.com>
// Copyright (c) 2021 Ross Light <ross@zombiezen.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
//
// SPDX-License-Identifier: ISC

package sqlite

import (
	"testing"
)

func TestFunc(t *testing.T) {
	c, err := OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = c.CreateFunction("addints", &FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(ctx Context, args []Value) (Value, error) {
			if got := ctx.Conn(); got != c {
				t.Errorf("ctx.Conn() = %p; want %p", got, c)
			}
			return IntegerValue(args[0].Int64() + args[1].Int64()), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stmt, _, err := c.PrepareTransient("SELECT addints(2, 3);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if got, want := stmt.ColumnInt(0), 5; got != want {
		t.Errorf("addints(2, 3)=%d, want %d", got, want)
	}
	stmt.Finalize()
}

func TestAggFunc(t *testing.T) {
	c, err := OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	stmt, _, err := c.PrepareTransient("CREATE TABLE t (c integer);")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}

	cVals := []int{3, 5, 7}
	want := 3 + 5 + 7

	stmt, err = c.Prepare("INSERT INTO t (c) VALUES ($c);")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Finalize()
	for _, val := range cVals {
		stmt.SetInt64("$c", int64(val))
		if _, err = stmt.Step(); err != nil {
			t.Errorf("INSERT %q: %v", val, err)
		}
		if err = stmt.Reset(); err != nil {
			t.Errorf("INSERT reset %q: %v", val, err)
		}
	}

	sumintsImpl := &FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		AllowIndirect: true,
	}
	{
		var sum int64
		sumintsImpl.AggregateStep = func(ctx Context, args []Value) {
			sum += args[0].Int64()
		}
		sumintsImpl.AggregateFinal = func(ctx Context) (Value, error) {
			result := IntegerValue(sum)
			sum = 0
			return result, nil
		}
	}
	if err := c.CreateFunction("sumints", sumintsImpl); err != nil {
		t.Fatal(err)
	}

	stmt, _, err = c.PrepareTransient("SELECT sum(c) FROM t;")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Finalize()
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if got := stmt.ColumnInt(0); got != want {
		t.Errorf("sum(c)=%d, want %d", got, want)
	}
}

func TestCastTextToInteger(t *testing.T) {
	tests := []struct {
		text string
		want int64
	}{
		{
			text: "abc",
			want: 0,
		},
		{
			text: "123",
			want: 123,
		},
		{
			text: " 123 ",
			want: 123,
		},
		{
			text: "+123",
			want: 123,
		},
		{
			text: "-123",
			want: -123,
		},
		{
			text: "123e+5",
			want: 123,
		},
		{
			text: "0x123",
			want: 0,
		},
		{
			text: "9223372036854775808",
			want: 9223372036854775807,
		},
		{
			text: "-9223372036854775809",
			want: -9223372036854775808,
		},
	}
	for _, test := range tests {
		if got := castTextToInteger(test.text); got != test.want {
			t.Errorf("castTextToInteger(%q) = %d; want %d", test.text, got, test.want)
		}
	}
}

func TestCastTextToReal(t *testing.T) {
	tests := []struct {
		text string
		want float64
	}{
		{
			text: "abc",
			want: 0,
		},
		{
			text: "123",
			want: 123,
		},
		{
			text: "123.45",
			want: 123.45,
		},
		{
			text: " 123.45 ",
			want: 123.45,
		},
		{
			text: "+123",
			want: 123,
		},
		{
			text: "-123",
			want: -123,
		},
		{
			text: "123e+5",
			want: 123e+5,
		},
		{
			text: "123.45xxx",
			want: 123.45,
		},
		{
			text: "0x123",
			want: 0,
		},
		{
			text: "9223372036854775808",
			want: 9223372036854775808,
		},
		{
			text: "-9223372036854775809",
			want: -9223372036854775809,
		},
	}
	for _, test := range tests {
		if got := castTextToReal(test.text); got != test.want {
			t.Errorf("castTextToReal(%q) = %g; want %g", test.text, got, test.want)
		}
	}
}

func TestIDGen(t *testing.T) {
	const newID = -1
	repeat := func(n int, seq ...int) []int {
		slice := make([]int, 0, len(seq)*n)
		for i := 0; i < n; i++ {
			slice = append(slice, seq...)
		}
		return slice
	}
	tests := []struct {
		name    string
		actions []int // non-negative means reclaim the ID at the given action index
	}{
		{
			name:    "Single",
			actions: []int{newID},
		},
		{
			name:    "LongSequence",
			actions: repeat(129, newID),
		},
		{
			name: "Reclaim",
			actions: []int{
				0: newID,
				1: 0,
				2: newID,
			},
		},
		{
			name: "ReclaimAfterAnother",
			actions: []int{
				0: newID,
				1: newID,
				2: 0,
				3: newID,
				4: newID,
			},
		},
		{
			name:    "LongSequenceWithMiddleReclaim",
			actions: append(repeat(129, newID), 42, newID),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gen := new(idGen)
			got := make([]uintptr, len(test.actions))
			used := make(map[uintptr]struct{})
			for i, reclaimIdx := range test.actions {
				if reclaimIdx < 0 {
					got[i] = gen.next()
					t.Logf("gen.next() = %d", got[i])
					if _, alreadyUsed := used[got[i]]; got[i] == 0 || alreadyUsed {
						t.Fail()
					}
					used[got[i]] = struct{}{}
				} else {
					id := got[reclaimIdx]
					t.Logf("gen.reclaim(%d)", id)
					gen.reclaim(id)
					delete(used, id)
				}
			}
		})
	}
}
