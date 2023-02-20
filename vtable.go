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

type WritableVTable interface {
	VTable
	Update(argv []Value) (rowID int64, err error)
}

// A TransactionVTable is a [VTable] that supports transactions.
type TransactionVTable interface {
	VTable

	// Begin begins a transaction on a virtual table.
	// Virtual table transactions do not nest,
	// so the Begin method will not be invoked more than once
	// on a single virtual table
	// without an intervening call to either Commit or Rollback.
	Begin() error
	// Sync signals the start of a two-phase commit on a virtual table.
	// This method is only invoked after a call to the Begin method
	// and prior to a Commit or Rollback.
	Sync() error
	// Commit causes a virtual table transaction to commit.
	Commit() error
	// Rollback causes a virtual table transaction to rollback.
	Rollback() error
}

// A SavepointVTable is a [VTable] that supports savepoints.
type SavepointVTable interface {
	TransactionVTable

	// Savepoint signals that the virtual table
	// should save its current state as savepoint N.
	Savepoint(n int) error
	// Release invalidates all savepoints greater than or equal to n.
	Release(n int) error
	// RollbackTo signals that the state of the virtual table
	// should return to what it was when Savepoint(n) was last called.
	// This invalidates all savepoints greater than n.
	RollbackTo(n int) error
}

type RenameVTable interface {
	VTable
	Rename(new string) error
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
	if cmod == 0 {
		return fmt.Errorf("sqlite: set module %q: %w", name, ResultNoMem.ToError())
	}
	libc.Xmemset(c.tls, cmod, 0, types.Size_t(unsafe.Sizeof(lib.Sqlite3_module{})))

	cmodPtr := (*lib.Sqlite3_module)(unsafe.Pointer(cmod))
	cmodPtr.FiVersion = 3
	if module.Create != nil {
		cmodPtr.FxCreate = cFuncPointer(vtabCreateTrampoline)
	}
	cmodPtr.FxConnect = cFuncPointer(vtabConnectTrampoline)
	cmodPtr.FxBestIndex = cFuncPointer(vtabBestIndexTrampoline)
	cmodPtr.FxDisconnect = cFuncPointer(vtabDisconnect)
	cmodPtr.FxDestroy = cFuncPointer(vtabDestroy)
	cmodPtr.FxOpen = cFuncPointer(vtabOpenTrampoline)
	cmodPtr.FxClose = cFuncPointer(vtabCloseTrampoline)
	cmodPtr.FxFilter = cFuncPointer(vtabFilterTrampoline)
	cmodPtr.FxNext = cFuncPointer(vtabNextTrampoline)
	cmodPtr.FxEof = cFuncPointer(vtabEOFTrampoline)
	cmodPtr.FxColumn = cFuncPointer(vtabColumnTrampoline)
	cmodPtr.FxRowid = cFuncPointer(vtabRowIDTrampoline)
	cmodPtr.FxBegin = cFuncPointer(vtabBeginTrampoline)
	cmodPtr.FxSync = cFuncPointer(vtabSyncTrampoline)
	cmodPtr.FxCommit = cFuncPointer(vtabCommitTrampoline)
	cmodPtr.FxRollback = cFuncPointer(vtabRollbackTrampoline)
	if module.CanRename {
		cmodPtr.FxRename = cFuncPointer(vtabRenameTrampoline)
	}
	cmodPtr.FxSavepoint = cFuncPointer(vtabSavepointTrampoline)
	cmodPtr.FxRelease = cFuncPointer(vtabReleaseTrampoline)
	cmodPtr.FxRollbackTo = cFuncPointer(vtabRollbackToTrampoline)

	xDestroy := cFuncPointer(destroyModule)

	xmodules.mu.Lock()
	defensiveCopy := new(Module)
	*defensiveCopy = *module
	// Module pointer address is unique for lifetime of module.
	xmodules.m[cmod] = defensiveCopy
	xmodules.mu.Unlock()

	res := ResultCode(lib.Xsqlite3_create_module_v2(c.tls, c.conn, cname, cmod, cmod, xDestroy))
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

	avt := assertVTable(vtab)
	xvtables.mu.Lock()
	id := xvtables.ids.next()
	xvtables.m[id] = avt
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
		Constraints: make([]IndexConstraint, info.FnConstraint),
		OrderBy:     make([]IndexOrderBy, info.FnOrderBy),
	}
	aConstraint := info.FaConstraint
	for i := range inputs.Constraints {
		c := (*lib.Sqlite3_index_constraint)(unsafe.Pointer(aConstraint))
		inputs.Constraints[i] = IndexConstraint{
			Column: int(c.FiColumn),
			Op:     IndexConstraintOp(c.Fop),
			Usable: c.Fusable != 0,
		}
		aConstraint += unsafe.Sizeof(lib.Sqlite3_index_constraint{})
	}
	aOrderBy := info.FaOrderBy
	for i := range inputs.OrderBy {
		o := (*lib.Sqlite3_index_orderby)(unsafe.Pointer(aOrderBy))
		inputs.OrderBy[i] = IndexOrderBy{
			Column: int(o.FiColumn),
			Desc:   o.Fdesc != 0,
		}
		aOrderBy += unsafe.Sizeof(lib.Sqlite3_index_orderby{})
	}

	outputs, err := vtab.BestIndex(inputs)
	if err != nil {
		return int32(ErrCode(err))
	}
	if len(outputs.ConstraintUsage) > int(info.FnConstraint) {
		return int32(ResultMisuse)
	}

	// Convert outputs from Go to C.
	aConstraintUsage := info.FaConstraintUsage
	for _, u := range outputs.ConstraintUsage {
		ptr := (*lib.Sqlite3_index_constraint_usage)(unsafe.Pointer(aConstraintUsage))
		ptr.FargvIndex = int32(u.ArgvIndex)
		if u.Omit {
			ptr.Fomit = 1
		} else {
			ptr.Fomit = 0
		}
		aConstraintUsage += unsafe.Sizeof(lib.Sqlite3_index_constraint_usage{})
	}
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

func vtabBeginTrampoline(tls *libc.TLS, pVTab uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Transaction != nil {
		if err := vtab.Transaction.Begin(); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabSyncTrampoline(tls *libc.TLS, pVTab uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Transaction != nil {
		if err := vtab.Transaction.Sync(); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabCommitTrampoline(tls *libc.TLS, pVTab uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Transaction != nil {
		if err := vtab.Transaction.Commit(); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabRollbackTrampoline(tls *libc.TLS, pVTab uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Transaction != nil {
		if err := vtab.Transaction.Rollback(); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabRenameTrampoline(tls *libc.TLS, pVTab uintptr, zNew uintptr) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Rename != nil {
		if err := vtab.Rename.Rename(libc.GoString(zNew)); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabSavepointTrampoline(tls *libc.TLS, pVTab uintptr, n int32) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Savepoint != nil {
		if err := vtab.Savepoint.Savepoint(int(n)); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabReleaseTrampoline(tls *libc.TLS, pVTab uintptr, n int32) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Savepoint != nil {
		if err := vtab.Savepoint.Release(int(n)); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func vtabRollbackToTrampoline(tls *libc.TLS, pVTab uintptr, n int32) int32 {
	vtabID := (*vtabWrapper)(unsafe.Pointer(pVTab)).id
	xvtables.mu.RLock()
	vtab := xvtables.m[vtabID]
	xvtables.mu.RUnlock()

	if vtab.Savepoint != nil {
		if err := vtab.Savepoint.RollbackTo(int(n)); err != nil {
			return int32(ErrCode(err))
		}
	}
	return lib.SQLITE_OK
}

func destroyModule(tls *libc.TLS, pAux uintptr) {
	xmodules.mu.Lock()
	delete(xmodules.m, pAux)
	xmodules.mu.Unlock()

	lib.Xsqlite3_free(tls, pAux)
}

type vtabWrapper struct {
	base lib.Sqlite3_vtab
	id   uintptr
}

type cursorWrapper struct {
	base lib.Sqlite3_vtab_cursor
	id   uintptr
}

type assertedVTable struct {
	VTable
	Write       WritableVTable
	Transaction TransactionVTable
	Savepoint   SavepointVTable
	Rename      RenameVTable
}

func assertVTable(vtab VTable) assertedVTable {
	avt := assertedVTable{VTable: vtab}
	avt.Write, _ = vtab.(WritableVTable)
	avt.Transaction, _ = vtab.(TransactionVTable)
	avt.Savepoint, _ = vtab.(SavepointVTable)
	avt.Rename, _ = vtab.(RenameVTable)
	return avt
}

var (
	xmodules = struct {
		mu sync.RWMutex
		m  map[uintptr]*Module
	}{
		m: make(map[uintptr]*Module),
	}
	xvtables = struct {
		mu  sync.RWMutex
		m   map[uintptr]assertedVTable
		ids idGen
	}{
		m: make(map[uintptr]assertedVTable),
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
