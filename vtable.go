// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package sqlite

type Module interface {
	Connect(c *Conn, argv []string) (VTable, error)
}

type VTable interface {
	// TODO(now): BestIndex(IndexInput) (IndexOutput, error)
	Open() (VTableCursor, error)
	Disconnect() error
	Destroy() error
}

type UpdateVTable interface {
	VTable
	Update(argv []Value) (rowID int64, err error)
}

type ShadowVTable interface {
	VTable
	ShadowName(string) bool
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

type VTableCursor interface {
	// TODO(now): Filter(idxNum int, idxStr string, argv []Value)
	Next() error
	Column(n int) (Value, error)
	RowID() (int64, error)
	EOF() bool
	Close() error
}
