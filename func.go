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
	"fmt"
	"sync"
	"unsafe"

	"modernc.org/libc"
	lib "modernc.org/sqlite/lib"
)

// Context is an *sqlite3_context.
// It is used by custom functions to return result values.
// An SQLite context is in no way related to a Go context.Context.
// https://sqlite.org/c3ref/context.html
type Context struct {
	tls *libc.TLS
	ptr uintptr
}

func (ctx Context) UserData() interface{} {
	return getxfuncs(ctx.tls, ctx.ptr).data
}

func (ctx Context) SetUserData(data interface{}) {
	getxfuncs(ctx.tls, ctx.ptr).data = data
}

func (ctx Context) ResultInt(v int)       { lib.Xsqlite3_result_int(ctx.tls, ctx.ptr, int32(v)) }
func (ctx Context) ResultInt64(v int64)   { lib.Xsqlite3_result_int64(ctx.tls, ctx.ptr, v) }
func (ctx Context) ResultFloat(v float64) { lib.Xsqlite3_result_double(ctx.tls, ctx.ptr, v) }
func (ctx Context) ResultNull()           { lib.Xsqlite3_result_null(ctx.tls, ctx.ptr) }
func (ctx Context) ResultValue(v Value)   { lib.Xsqlite3_result_value(ctx.tls, ctx.ptr, v.ptr) }

func (ctx Context) ResultZeroBlob(n int64) {
	lib.Xsqlite3_result_zeroblob64(ctx.tls, ctx.ptr, uint64(n))
}

func (ctx Context) ResultText(v string) {
	var cv uintptr
	if len(v) == 0 {
		cv = emptyCString
	} else {
		var err error
		cv, err = libc.CString(v)
		if err != nil {
			panic(err)
		}
	}
	lib.Xsqlite3_result_text(ctx.tls, ctx.ptr, cv, int32(len(v)), freeFuncPtr)
}

func (ctx Context) ResultError(err error) {
	errstr := err.Error()
	cerrstr, err := libc.CString(errstr)
	if err != nil {
		panic(err)
	}
	defer libc.Xfree(ctx.tls, cerrstr)
	lib.Xsqlite3_result_error(ctx.tls, ctx.ptr, cerrstr, int32(len(errstr)))
	if code := ErrCode(err); code != ResultError {
		lib.Xsqlite3_result_error_code(ctx.tls, ctx.ptr, int32(code))
	}
}

type Value struct {
	tls *libc.TLS
	ptr uintptr
}

func (v Value) IsNil() bool      { return v.ptr == 0 }
func (v Value) Int() int         { return int(lib.Xsqlite3_value_int(v.tls, v.ptr)) }
func (v Value) Int64() int64     { return int64(lib.Xsqlite3_value_int64(v.tls, v.ptr)) }
func (v Value) Float() float64   { return float64(lib.Xsqlite3_value_double(v.tls, v.ptr)) }
func (v Value) Len() int         { return int(lib.Xsqlite3_value_bytes(v.tls, v.ptr)) }
func (v Value) Type() ColumnType { return ColumnType(lib.Xsqlite3_value_type(v.tls, v.ptr)) }
func (v Value) Text() string     { return goStringN(lib.Xsqlite3_value_text(v.tls, v.ptr), v.Len()) }
func (v Value) Blob() []byte     { return libc.GoBytes(lib.Xsqlite3_value_blob(v.tls, v.ptr), v.Len()) }

type xfunc struct {
	id     int
	name   string
	conn   *Conn
	xFunc  func(Context, ...Value)
	xStep  func(Context, ...Value)
	xFinal func(Context)
	data   interface{}
}

var xfuncs = struct {
	mu   sync.RWMutex
	m    map[int]*xfunc
	next int
}{
	m: make(map[int]*xfunc),
}

// CreateFunction registers a Go function with SQLite
// for use in SQL queries.
//
// To define a scalar function, provide a value for
// xFunc and set xStep/xFinal to nil.
//
// To define an aggregation set xFunc to nil and
// provide values for xStep and xFinal.
//
// State can be stored across function calls by
// using the Context UserData/SetUserData methods.
//
// https://sqlite.org/c3ref/create_function.html
func (c *Conn) CreateFunction(name string, deterministic bool, numArgs int, xFunc, xStep func(Context, ...Value), xFinal func(Context)) error {
	cname, err := libc.CString(name)
	if err != nil {
		return fmt.Errorf("sqlite: create function %s: %w", name, err)
	}
	defer libc.Xfree(c.tls, cname)

	eTextRep := int32(lib.SQLITE_UTF8)
	if deterministic {
		eTextRep |= lib.SQLITE_DETERMINISTIC
	}

	x := &xfunc{
		conn:   c,
		name:   name,
		xFunc:  xFunc,
		xStep:  xStep,
		xFinal: xFinal,
	}

	xfuncs.mu.Lock()
	xfuncs.next++
	x.id = xfuncs.next
	xfuncs.m[x.id] = x
	xfuncs.mu.Unlock()

	pApp := uintptr(x.id)

	// The following are conversions from function values to uintptr. It assumes
	// the memory representation described in https://golang.org/s/go11func.
	//
	// It does this by doing the following in order:
	// 1) Create a Go struct containing a pointer to a pointer to
	//    the function. It is assumed that the pointer to the function will be
	//    stored in the read-only data section and thus will not move.
	// 2) Convert the pointer to the Go struct to a pointer to uintptr through
	//    unsafe.Pointer. This is permitted via Rule #1 of unsafe.Pointer.
	// 3) Dereference the pointer to uintptr to obtain the function value as a
	//    uintptr. This is safe as long as function values are passed as pointers.
	var funcfn, stepfn, finalfn uintptr
	if xFunc == nil {
		stepfn = *(*uintptr)(unsafe.Pointer(&struct {
			f func(*libc.TLS, uintptr, int32, uintptr)
		}{stepTramp}))
		finalfn = *(*uintptr)(unsafe.Pointer(&struct {
			f func(*libc.TLS, uintptr)
		}{finalTramp}))
	} else {
		funcfn = *(*uintptr)(unsafe.Pointer(&struct {
			f func(*libc.TLS, uintptr, int32, uintptr)
		}{funcTramp}))
	}
	destroyfn := *(*uintptr)(unsafe.Pointer(&struct {
		f func(*libc.TLS, uintptr)
	}{goDestroyTramp}))

	res := ResultCode(lib.Xsqlite3_create_function_v2(
		c.tls,
		c.conn,
		cname,
		int32(numArgs),
		eTextRep,
		pApp,
		funcfn,
		stepfn,
		finalfn,
		destroyfn,
	))
	if err := reserr(res); err != nil {
		return fmt.Errorf("sqlite: create function %s: %w", name, err)
	}
	return nil
}

func getxfuncs(tls *libc.TLS, ctx uintptr) *xfunc {
	id := int(lib.Xsqlite3_user_data(tls, ctx))

	xfuncs.mu.RLock()
	x := xfuncs.m[id]
	xfuncs.mu.RUnlock()

	return x
}

func funcTramp(tls *libc.TLS, ctx uintptr, n int32, valarray uintptr) {
	vals := make([]Value, 0, int(n))
	for ; len(vals) < cap(vals); valarray += uintptr(ptrSize) {
		vals = append(vals, Value{
			tls: tls,
			ptr: *(*uintptr)(unsafe.Pointer(valarray)),
		})
	}
	getxfuncs(tls, ctx).xFunc(Context{tls: tls, ptr: ctx}, vals...)
}

func stepTramp(tls *libc.TLS, ctx uintptr, n int32, valarray uintptr) {
	vals := make([]Value, 0, int(n))
	for ; len(vals) < cap(vals); valarray += uintptr(ptrSize) {
		vals = append(vals, Value{
			tls: tls,
			ptr: *(*uintptr)(unsafe.Pointer(valarray)),
		})
	}
	getxfuncs(tls, ctx).xStep(Context{tls: tls, ptr: ctx}, vals...)
}

func finalTramp(tls *libc.TLS, ctx uintptr) {
	x := getxfuncs(tls, ctx)
	x.xFinal(Context{tls: tls, ptr: ctx})
}

func goDestroyTramp(tls *libc.TLS, ptr uintptr) {
	id := int(ptr)

	xfuncs.mu.Lock()
	delete(xfuncs.m, id)
	xfuncs.mu.Unlock()
}
