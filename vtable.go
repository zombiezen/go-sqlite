// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	lib "modernc.org/sqlite/lib"
)

// A Module declares a [virtual table] that can be registered with a [Conn]
// via [Conn.SetModule].
//
// [virtual table]: https://sqlite.org/vtab.html
type Module struct {
	Connect   VTableConnectFunc
	Create    VTableConnectFunc
	Writable  bool
	CanRename bool
}

type VTableConnectFunc func(c *Conn, argv []string) (VTable, *VTableConfig, error)

// VTableConfig specifies the configuration of a [VTable] returned by [VTableConnectFunc].
// [VTableConfig.Declaration] is the only required field.
type VTableConfig struct {
	// Declaration must be a [CREATE TABLE statement]
	// that defines the columns in the virtual table and their data type.
	// The name of the table in this CREATE TABLE statement is ignored,
	// as are all constraints.
	//
	// [CREATE TABLE statement]: https://sqlite.org/lang_createtable.html
	Declaration string

	// If ConstraintSupport is true, then the virtual table implementation
	// guarantees that if Update returns a [ResultConstraint] error,
	// it will do so before any modifications to internal or persistent data structures
	// have been made.
	ConstraintSupport bool

	// If AllowIndirect is false, then the virtual table may only be used from top-level SQL.
	// If AllowIndirect is true, then the virtual table can be used in VIEWs, TRIGGERs,
	// and schema structures (e.g. CHECK constraints and DEFAULT clauses).
	//
	// This is the inverse of SQLITE_DIRECTONLY.
	// See https://sqlite.org/c3ref/c_vtab_constraint_support.html
	// for more details.
	// This defaults to false for better security.
	AllowIndirect bool
}

// VTable represents a connected [virtual table].
//
// [virtual table]: https://sqlite.org/vtab.html
type VTable interface {
	BestIndex(*IndexInputs) (*IndexOutputs, error)
	Open() (VTableCursor, error)
	Disconnect() error
	Destroy() error
}

type WriteableVTable interface {
	VTable
	Update(argv []Value) (rowID int64, err error)
}

type TransactionVTable interface {
	VTable
	Begin() error
	Sync() error
	Commit() error
	Rollback() error
}

type SavepointVTable interface {
	TransactionVTable
	Savepoint(n int) error
	Release(n int) error
	RollbackTo(n int) error
}

type RenameVTable interface {
	VTable
	Rename(new string) error
}

type ShadowVTable interface {
	VTable
	ShadowName(string) bool
}

type IndexInputs struct {
	// Constraints corresponds to the WHERE clause.
	Constraints []IndexConstraint
	// OrderBy corresponds to the ORDER BY clause.
	OrderBy []IndexOrderBy
	// ColumnsUsed is a bitmask of columns used by the statement.
	ColumnsUsed uint64
}

type IndexConstraint struct {
	Column int
	Op     IndexConstraintOp
	Usable bool
}

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

	IndexConstraintFunction IndexConstraintOp = lib.SQLITE_INDEX_CONSTRAINT_FUNCTION
)

type IndexOrderBy struct {
	Column int
	Desc   bool
}

type IndexOutputs struct {
	// ConstraintUsage is a mapping from [IndexInputs.Constraints]
	// to [VTableCursor.Filter] arguments.
	// The mapping is in the same order as [IndexInputs.Constraints]
	// and must not contain more than len(IndexInputs.Constraints) elements.
	// If len(ConstraintUsage) < len(IndexInputs.Constraints),
	// then ConstraintUsage is treated as if the missing elements have the zero value.
	ConstraintUsage []IndexConstraintUsage
	// ID is used to identify the index in [VTableCursor.Filter].
	ID IndexID
	// OrderByConsumed is true if the output is already ordered.
	OrderByConsumed bool
	// EstimatedCost is an estimate of the cost of a particular strategy.
	// A cost of N indicates that the cost of the strategy
	// is similar to a linear scan of an SQLite table with N rows.
	// A cost of log(N) indicates that the expense of the operation
	// is similar to that of a binary search on a unique indexed field
	// of an SQLite table with N rows.
	EstimatedCost float64
	// EstimatedRows is an estimate of the number of rows
	// that will be returned by the strategy.
	EstimatedRows int64
	// IndexFlags is a bitmask of other flags about the index.
	IndexFlags IndexFlags
}

// IndexConstraintUsage maps a single constraint from [IndexInputs.Constraints]
// to a [VTableCursor.Filter] argument in the [IndexOutputs.ConstraintUsage] list.
type IndexConstraintUsage struct {
	// ArgvIndex is the intended [VTableCursor.Filter] argument index plus one.
	// If ArgvIndex is zero or negative,
	// then the constraint is not passed to [VTableCursor.Filter].
	// Within the [IndexOutputs.ConstraintUsage] list,
	// there must be exactly one entry with an ArgvIndex of 1,
	// another of 2, another of 3, and so forth
	// to as many or as few as the [VTable.BestIndex] method wants.
	ArgvIndex int
	// If Omit is true, then it is a hint to SQLite
	// that the virtual table will guarantee that the constraint will always be satisfied.
	// SQLite will always double-check that rows satisfy the constraint if Omit is false,
	// but may skip this check if Omit is false.
	Omit bool
}

// IndexID is a virtual table index identifier.
// The meaning of its fields is defined by the virtual table implementation.
type IndexID struct {
	Num    int32
	String string
}

// IndexFlags is a bitmap of options returned in [IndexOutputs.IndexFlags].
type IndexFlags uint32

const (
	IndexScanUnique IndexFlags = lib.SQLITE_INDEX_SCAN_UNIQUE
)

type VTableCursor interface {
	Filter(id IndexID, argv []Value) error
	Next() error
	Column(i int) (Value, error)
	RowID() (int64, error)
	EOF() bool
	Close() error
}

// SetModule registers or unregisters a virtual table module with the given name.
func (c *Conn) SetModule(name string, module *Module) error {
	if c == nil {
		return fmt.Errorf("sqlite: set module %q: nil connection", name)
	}
	cname, err := libc.CString(name)
	if err != nil {
		return fmt.Errorf("sqlite: set module %q: %v", name, err)
	}
	defer libc.Xfree(c.tls, cname)

	if module == nil {
		res := ResultCode(lib.Xsqlite3_create_module_v2(c.tls, c.conn, cname, 0, 0, 0))
		if err := res.ToError(); err != nil {
			return fmt.Errorf("sqlite: set module %q: %w", name, err)
		}
		return nil
	}
	if module.Connect == nil {
		return fmt.Errorf("sqlite: set module %q: connect not provided", name)
	}

	cmod := lib.Xsqlite3_malloc(c.tls, int32(unsafe.Sizeof(lib.Sqlite3_module{})))
	cmodPtr := (*lib.Sqlite3_module)(unsafe.Pointer(cmod))
	cmodPtr.FiVersion = 3

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
	if module.Create != nil {
		cmodPtr.FxCreate = *(*uintptr)(unsafe.Pointer(&struct {
			f func(tls *libc.TLS, db uintptr, pAux uintptr, argc int32, argv uintptr, ppVTab uintptr, pzErr uintptr) int32
		}{vtabCreateTrampoline}))
	}
	cmodPtr.FxConnect = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, db uintptr, pAux uintptr, argc int32, argv uintptr, ppVTab uintptr, pzErr uintptr) int32
	}{vtabConnectTrampoline}))
	cmodPtr.FxBestIndex = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pVTab uintptr, info uintptr) int32
	}{vtabBestIndexTrampoline}))
	cmodPtr.FxDisconnect = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pVTab uintptr) int32
	}{vtabDisconnect}))
	cmodPtr.FxDestroy = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pVTab uintptr) int32
	}{vtabDestroy}))
	cmodPtr.FxOpen = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pVTab uintptr, ppCursor uintptr) int32
	}{vtabOpenTrampoline}))
	cmodPtr.FxClose = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr) int32
	}{vtabCloseTrampoline}))
	cmodPtr.FxFilter = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr, idxNum int32, idxStr uintptr, argc int32, argv uintptr) int32
	}{vtabFilterTrampoline}))
	cmodPtr.FxNext = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr) int32
	}{vtabNextTrampoline}))
	cmodPtr.FxEof = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr) int32
	}{vtabEOFTrampoline}))
	cmodPtr.FxColumn = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr, ctx uintptr, n int32) int32
	}{vtabColumnTrampoline}))
	cmodPtr.FxRowid = *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pCursor uintptr, pRowid uintptr) int32
	}{vtabRowIDTrampoline}))

	xDestroy := *(*uintptr)(unsafe.Pointer(&struct {
		f func(tls *libc.TLS, pAux uintptr)
	}{destroyModule}))

	xmodules.mu.Lock()
	id := xmodules.ids.next()
	defensiveCopy := new(Module)
	*defensiveCopy = *module
	xmodules.m[id] = defensiveCopy
	xmodules.mu.Unlock()

	res := ResultCode(lib.Xsqlite3_create_module_v2(c.tls, c.conn, cname, cmod, id, xDestroy))
	if err := res.ToError(); err != nil {
		return fmt.Errorf("sqlite: set module %q: %w", name, err)
	}
	return nil
}

func vtabCreateTrampoline(tls *libc.TLS, db uintptr, pAux uintptr, argc int32, argv uintptr, ppVTab uintptr, pzErr uintptr) int32 {
	xmodules.mu.RLock()
	module := xmodules.m[pAux]
	xmodules.mu.RUnlock()
	return callConnectFunc(tls, module.Create, db, argc, argv, ppVTab, pzErr)
}

func vtabConnectTrampoline(tls *libc.TLS, db uintptr, pAux uintptr, argc int32, argv uintptr, ppVTab uintptr, pzErr uintptr) int32 {
	xmodules.mu.RLock()
	module := xmodules.m[pAux]
	xmodules.mu.RUnlock()
	return callConnectFunc(tls, module.Connect, db, argc, argv, ppVTab, pzErr)
}

func callConnectFunc(tls *libc.TLS, connect VTableConnectFunc, db uintptr, argc int32, argv uintptr, ppVTab uintptr, pzErr uintptr) int32 {
	allConns.mu.RLock()
	c := allConns.table[db]
	allConns.mu.RUnlock()

	goArgv := make([]string, argc)
	for i := range goArgv {
		goArgv[i] = libc.GoString(*(*uintptr)(unsafe.Pointer(argv)))
		argv += uintptr(ptrSize)
	}

	vtab, cfg, err := connect(c, goArgv)
	if err != nil {
		zerr, _ := sqliteCString(tls, err.Error())
		*(*uintptr)(unsafe.Pointer(pzErr)) = zerr
		return int32(ErrCode(err))
	}
	cdecl, err := libc.CString(cfg.Declaration)
	if err != nil {
		vtab.Disconnect()
		return lib.SQLITE_NOMEM
	}
	defer libc.Xfree(tls, cdecl)
	res := ResultCode(lib.Xsqlite3_declare_vtab(tls, db, cdecl))
	if !res.IsSuccess() {
		vtab.Disconnect()
		return int32(res)
	}
	// TODO(now): other cfg options

	vtabWrapperSize := int32(unsafe.Sizeof(vtabWrapper{}))
	pvtab := lib.Xsqlite3_malloc(tls, vtabWrapperSize)
	*(*uintptr)(unsafe.Pointer(ppVTab)) = pvtab
	if pvtab == 0 {
		vtab.Disconnect()
		return lib.SQLITE_NOMEM
	}
	libc.Xmemset(tls, pvtab, 0, types.Size_t(vtabWrapperSize))

	xvtables.mu.Lock()
	id := xvtables.ids.next()
	xvtables.m[id] = vtab
	xvtables.mu.Unlock()
	(*vtabWrapper)(unsafe.Pointer(pvtab)).id = id

	return lib.SQLITE_OK
}

func vtabDisconnect(tls *libc.TLS, pVTab uintptr) int32 {
	id := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	lib.Xsqlite3_free(tls, pVTab)

	xvtables.mu.Lock()
	xvtables.ids.reclaim(id)
	vtab := xvtables.m[id]
	delete(xvtables.m, id)
	xvtables.mu.Unlock()

	return int32(ErrCode(vtab.Disconnect()))
}

func vtabDestroy(tls *libc.TLS, pVTab uintptr) int32 {
	id := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	lib.Xsqlite3_free(tls, pVTab)

	xvtables.mu.Lock()
	xvtables.ids.reclaim(id)
	vtab := xvtables.m[id]
	delete(xvtables.m, id)
	xvtables.mu.Unlock()

	return int32(ErrCode(vtab.Destroy()))
}

func vtabBestIndexTrampoline(tls *libc.TLS, pVTab uintptr, infoPtr uintptr) int32 {
	id := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	info := (*lib.Sqlite3_index_info)(unsafe.Pointer(infoPtr))
	xvtables.mu.RLock()
	vtab := xvtables.m[id]
	xvtables.mu.RUnlock()

	// Convert inputs from C to Go.
	inputs := &IndexInputs{
		ColumnsUsed: info.FcolUsed,
		// TODO(soon)
		// Constraints: make([]IndexConstraint, info.FnConstraint),
		// OrderBy:     make([]IndexOrderBy, info.FnOrderBy),
	}

	outputs, err := vtab.BestIndex(inputs)
	if err != nil {
		return int32(ErrCode(err))
	}

	// Convert outputs from Go to C.
	info.FidxNum = outputs.ID.Num
	if len(outputs.ID.String) == 0 {
		info.FidxStr = 0
		info.FneedToFreeIdxStr = 0
	} else {
		var err error
		info.FidxStr, err = sqliteCString(tls, outputs.ID.String)
		if err != nil {
			return int32(ErrCode(err))
		}
		info.FneedToFreeIdxStr = 1
	}
	if outputs.OrderByConsumed {
		info.ForderByConsumed = 1
	} else {
		info.ForderByConsumed = 0
	}
	info.FestimatedCost = outputs.EstimatedCost
	info.FestimatedRows = outputs.EstimatedRows
	info.FidxFlags = int32(outputs.IndexFlags)
	// TODO(soon): ConstraintUsage

	return lib.SQLITE_OK
}

func vtabOpenTrampoline(tls *libc.TLS, pVTab uintptr, ppCursor uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	cursor, err := vtab.Open()
	if err != nil {
		return int32(ErrCode(err))
	}

	cursorWrapperSize := int32(unsafe.Sizeof(vtabWrapper{}))
	pcursor := lib.Xsqlite3_malloc(tls, cursorWrapperSize)
	*(*uintptr)(unsafe.Pointer(ppCursor)) = pcursor
	if pcursor == 0 {
		cursor.Close()
		return lib.SQLITE_NOMEM
	}
	libc.Xmemset(tls, pcursor, 0, types.Size_t(cursorWrapperSize))

	xcursors.mu.Lock()
	cursorID := xcursors.ids.next()
	xcursors.m[cursorID] = cursor
	xcursors.mu.Unlock()
	(*cursorWrapper)(unsafe.Pointer(pcursor)).id = cursorID

	return lib.SQLITE_OK
}

func vtabCloseTrampoline(tls *libc.TLS, pCursor uintptr) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.Lock()
	cur := xcursors.m[id]
	delete(xcursors.m, id)
	xcursors.ids.reclaim(id)
	xcursors.mu.Unlock()

	lib.Xsqlite3_free(tls, pCursor)
	if err := cur.Close(); err != nil {
		return int32(ErrCode(err))
	}
	return lib.SQLITE_OK
}

func vtabFilterTrampoline(tls *libc.TLS, pCursor uintptr, idxNum int32, idxStr uintptr, argc int32, argv uintptr) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.RLock()
	cur := xcursors.m[id]
	xcursors.mu.RUnlock()

	idxID := IndexID{
		Num:    idxNum,
		String: libc.GoString(idxStr),
	}
	goArgv := make([]Value, 0, int(argc))
	for ; len(goArgv) < cap(goArgv); argv += uintptr(ptrSize) {
		goArgv = append(goArgv, Value{
			tls:       tls,
			ptrOrType: *(*uintptr)(unsafe.Pointer(argv)),
		})
	}
	if err := cur.Filter(idxID, goArgv); err != nil {
		return int32(ErrCode(err))
	}
	return lib.SQLITE_OK
}

func vtabNextTrampoline(tls *libc.TLS, pCursor uintptr) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.RLock()
	cur := xcursors.m[id]
	xcursors.mu.RUnlock()

	if err := cur.Next(); err != nil {
		return int32(ErrCode(err))
	}
	return lib.SQLITE_OK
}

func vtabEOFTrampoline(tls *libc.TLS, pCursor uintptr) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.RLock()
	cur := xcursors.m[id]
	xcursors.mu.RUnlock()

	if cur.EOF() {
		return 1
	}
	return 0
}

func vtabColumnTrampoline(tls *libc.TLS, pCursor uintptr, ctx uintptr, n int32) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.RLock()
	cur := xcursors.m[id]
	xcursors.mu.RUnlock()

	goCtx := Context{tls: tls, ptr: ctx}
	v, err := cur.Column(int(n))
	if err != nil {
		goCtx.result(TextValue(err.Error()), nil)
		return int32(ErrCode(err))
	}

	goCtx.result(v, nil)
	return lib.SQLITE_OK
}

func vtabRowIDTrampoline(tls *libc.TLS, pCursor uintptr, pRowid uintptr) int32 {
	id := (*cursorWrapper)(unsafe.Pointer(pCursor)).id
	xcursors.mu.RLock()
	cur := xcursors.m[id]
	xcursors.mu.RUnlock()

	rowID, err := cur.RowID()
	if err != nil {
		return int32(ErrCode(err))
	}
	*(*int64)(unsafe.Pointer(pRowid)) = rowID
	return lib.SQLITE_OK
}

func destroyModule(tls *libc.TLS, pAux uintptr) {
	xmodules.mu.Lock()
	xmodules.ids.reclaim(pAux)
	delete(xmodules.m, pAux)
	xmodules.mu.Unlock()
}

type vtabWrapper struct {
	base lib.Sqlite3_vtab
	id   uintptr
}

type cursorWrapper struct {
	base lib.Sqlite3_vtab_cursor
	id   uintptr
}

var (
	xmodules = struct {
		mu  sync.RWMutex
		m   map[uintptr]*Module
		ids idGen
	}{
		m: make(map[uintptr]*Module),
	}
	xvtables = struct {
		mu  sync.RWMutex
		m   map[uintptr]VTable
		ids idGen
	}{
		m: make(map[uintptr]VTable),
	}
	xcursors = struct {
		mu  sync.RWMutex
		m   map[uintptr]VTableCursor
		ids idGen
	}{
		m: make(map[uintptr]VTableCursor),
	}
)

// sqliteCString copies a Go string to SQLite-allocated memory.
func sqliteCString(tls *libc.TLS, s string) (uintptr, error) {
	if strings.Contains(s, "\x00") {
		return 0, fmt.Errorf("%q contains NUL bytes", s)
	}
	csize := len(s) + 1
	c := lib.Xsqlite3_malloc(tls, int32(csize))
	if c == 0 {
		return 0, fmt.Errorf("%w: cannot allocate %d bytes", ResultNoMem.ToError(), len(s))
	}
	cslice := unsafe.Slice((*byte)(unsafe.Pointer(c)), csize)
	copy(cslice, s)
	cslice[len(s)] = 0
	return c, nil
}
