// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Package sqlitemigration provides a connection pool type that guarantees a
// series of SQL scripts has been run once successfully before making
// connections available to the application. This is frequently useful for
// ensuring tables are created.
package sqlitemigration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Schema defines the migrations for the application.
type Schema struct {
	// AppID is saved to the database file to identify the application.
	// It's used to prevent opening database files for a different
	// application. It should be a positive number, but should not change
	// between runs of the same program. A common way of setting this is with
	// a compile-time constant that was randomly generated.
	AppID int32

	// Migrations is a list of SQL scripts to run. Each script is wrapped in a
	// transaction which is rolled back on any error.
	Migrations []string

	// RepeatableMigration is a SQL script to run if any migrations ran. The
	// script is wrapped in a transaction which is rolled back on any error.
	RepeatableMigration string
}

// Options specifies optional behaviors for the pool.
type Options struct {
	// Flags is interpreted the same way as the argument to sqlitex.Open.
	Flags sqlite.OpenFlags

	// PoolSize sets an explicit size to the pool. If less than 1, a reasonable
	// default is used.
	PoolSize int

	// OnStartMigrate is called after the pool has successfully opened a
	// connection to the database but before any migrations have been run.
	OnStartMigrate SignalFunc
	// OnReady is called after the pool has connected to the database and run any
	// necessary migrations.
	OnReady SignalFunc
	// OnError is called when the pool encounters errors while applying the
	// migration. This is typically used for logging errors.
	OnError ReportFunc

	// PrepareConn is called for each connection in the pool to set up functions
	// and other connection-specific state.
	PrepareConn ConnPrepareFunc
}

func (opts Options) realPoolSize() int {
	if opts.PoolSize < 1 {
		return 10
	}
	return opts.PoolSize
}

// Pool is a pool of SQLite connections.
type Pool struct {
	retry  chan struct{}
	opts   Options
	cancel context.CancelFunc

	initedMu sync.RWMutex // protects inited
	inited   map[*sqlite.Conn]struct{}

	ready <-chan struct{} // protects the following fields
	pool  *sqlitex.Pool
	err   error

	closedMu sync.RWMutex
	closed   bool
}

// NewPool opens a new pool of SQLite connections.
func NewPool(uri string, schema Schema, opts Options) *Pool {
	ready := make(chan struct{})
	retry := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		retry:  retry,
		opts:   opts,
		cancel: cancel,
		ready:  ready,
	}
	if opts.PrepareConn != nil {
		p.inited = make(map[*sqlite.Conn]struct{})
	}
	go func() {
		defer close(ready)
		defer cancel()
		p.pool, p.err = p.open(ctx, uri, schema)
		if p.err != nil {
			opts.OnError.call(p.err)
		}
	}()
	return p
}

// Close closes all connections in the Pool, potentially interrupting
// a migration.
func (p *Pool) Close() error {
	p.closedMu.Lock()
	if p.closed {
		p.closedMu.Unlock()
		return errors.New("close sqlite pool: already closed")
	}
	p.closed = true
	p.closedMu.Unlock()

	p.cancel()
	<-p.ready
	if p.pool == nil {
		return nil
	}
	return p.pool.Close()
}

// Get gets an SQLite connection from the pool.
func (p *Pool) Get(ctx context.Context) (*sqlite.Conn, error) {
	tick := time.NewTicker(5 * time.Second)
	for ready := false; !ready; {
		// Inform Pool.open to keep trying.
		select {
		case p.retry <- struct{}{}:
		default:
		}

		select {
		case <-tick.C:
			// Another try.
		case <-p.ready:
			ready = true
		case <-ctx.Done():
			tick.Stop()
			return nil, fmt.Errorf("get sqlite conn: %w", ctx.Err())
		}
	}
	tick.Stop()

	if p.err != nil {
		return nil, fmt.Errorf("get sqlite conn: %w", p.err)
	}
	conn := p.pool.Get(ctx)
	if conn == nil {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("get sqlite conn: %w", err)
		}
		return nil, errors.New("get sqlite conn: pool closed")
	}
	if err := p.prepare(conn); err != nil {
		p.pool.Put(conn)
		return nil, fmt.Errorf("get sqlite conn: %w", err)
	}
	return conn, nil
}

func (p *Pool) prepare(conn *sqlite.Conn) error {
	if p.opts.PrepareConn == nil {
		return nil
	}
	p.initedMu.RLock()
	_, inited := p.inited[conn]
	p.initedMu.RUnlock()
	if inited {
		return nil
	}
	if err := p.opts.PrepareConn(conn); err != nil {
		return fmt.Errorf("prepare connection: %w", err)
	}
	// This will not race, since other goroutines will not be able to acquire the
	// connection from the pool.
	p.initedMu.Lock()
	p.inited[conn] = struct{}{}
	p.initedMu.Unlock()
	return nil
}

// Put puts an SQLite connection back into the pool.
// See sqlitex.Pool for details.
func (p *Pool) Put(conn *sqlite.Conn) {
	select {
	case <-p.ready:
	default:
		panic("Pool.Put before pool is ready")
	}
	if p.err != nil {
		panic("Pool.Put on failed pool")
	}
	p.pool.Put(conn)
}

// CheckHealth returns an error if the migration has not completed.
// Closed pools may report healthy.
func (p *Pool) CheckHealth() error {
	p.closedMu.RLock()
	closed := p.closed
	p.closedMu.RUnlock()
	if closed {
		return errors.New("sqlite pool health: closed")
	}

	select {
	case <-p.ready:
		if p.err != nil {
			return fmt.Errorf("sqlite pool health: %w", p.err)
		}
		return nil
	default:
		return errors.New("sqlite pool health: not ready")
	}
}

func (p *Pool) open(ctx context.Context, uri string, schema Schema) (*sqlitex.Pool, error) {
	for first := true; ; first = false {
		if !first {
			select {
			case <-p.retry:
			case <-ctx.Done():
				return nil, errors.New("closed before successful migration")
			}
		}

		pool, err := sqlitex.Open(uri, p.opts.Flags, p.opts.realPoolSize())
		if err != nil {
			p.opts.OnError.call(err)
			continue
		}
		conn := pool.Get(ctx)
		if conn == nil {
			// Canceled.
			pool.Close()
			return nil, errors.New("closed before successful migration")
		}
		if err := p.prepare(conn); err != nil {
			pool.Put(conn)
			if closeErr := pool.Close(); closeErr != nil {
				p.opts.OnError.call(fmt.Errorf("close after failed connection preparation: %w", closeErr))
			}
			return nil, err
		}
		err = migrateDB(ctx, conn, schema, p.opts.OnStartMigrate)
		pool.Put(conn)
		if err != nil {
			if closeErr := pool.Close(); closeErr != nil {
				p.opts.OnError.call(fmt.Errorf("close after failed migration: %w", closeErr))
			}
			return nil, err
		}
		p.opts.OnReady.call()
		return pool, nil
	}
}

// Migrate performs any unapplied migrations in the schema on the database.
func Migrate(ctx context.Context, conn *sqlite.Conn, schema Schema) error {
	return migrateDB(ctx, conn, schema, nil)
}

func migrateDB(ctx context.Context, conn *sqlite.Conn, schema Schema, onStart SignalFunc) (err error) {
	defer conn.SetInterrupt(conn.SetInterrupt(ctx.Done()))
	defer sqlitex.Save(conn)(&err)

	var hasSchema bool
	err = sqlitex.ExecTransient(conn, "VALUES ((SELECT COUNT(*) FROM sqlite_master) > 0);", func(stmt *sqlite.Stmt) error {
		hasSchema = stmt.ColumnInt(0) != 0
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	var dbAppID int32
	err = sqlitex.ExecTransient(conn, "PRAGMA application_id;", func(stmt *sqlite.Stmt) error {
		dbAppID = stmt.ColumnInt32(0)
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if dbAppID != schema.AppID && !(dbAppID == 0 && !hasSchema) {
		return fmt.Errorf("migrate database: database application_id = %#x (expected %#x)", dbAppID, schema.AppID)
	}
	var schemaVersion int
	err = sqlitex.ExecTransient(conn, "PRAGMA user_version;", func(stmt *sqlite.Stmt) error {
		schemaVersion = stmt.ColumnInt(0)
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	// Using Sprintf because PRAGMAs don't permit arbitrary expressions, and thus
	// don't permit using parameter substitution.
	err = sqlitex.ExecTransient(conn, fmt.Sprintf("PRAGMA application_id = %d;", schema.AppID), nil)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	onStart.call()
	migrated := schemaVersion < len(schema.Migrations)
	for ; schemaVersion < len(schema.Migrations); schemaVersion++ {
		migration := schema.Migrations[schemaVersion]
		if migration == "" {
			continue
		}
		err := sqlitex.ExecScript(conn, fmt.Sprintf("%s;\nPRAGMA user_version = %d;\n", migration, schemaVersion+1))
		if err != nil {
			return fmt.Errorf("migrate database: apply migrations[%d]: %w", schemaVersion, err)
		}
	}
	if migrated && schema.RepeatableMigration != "" {
		if err := sqlitex.ExecScript(conn, schema.RepeatableMigration); err != nil {
			return fmt.Errorf("migrate database: apply repeatable migration: %w", err)
		}
	}
	return nil
}

// A SignalFunc is called at most once when a particular event in a Pool's
// lifecycle occurs.
type SignalFunc func()

func (f SignalFunc) call() {
	if f == nil {
		return
	}
	f()
}

// A ReportFunc is called for transient errors the pool encounters while
// running the migrations. It must be safe to call from multiple goroutines.
type ReportFunc func(error)

func (f ReportFunc) call(err error) {
	if f == nil {
		return
	}
	f(err)
}

// A ConnPrepareFunc is called for each connection in a pool to set up
// connection-specific state. It must be safe to call from multiple goroutines.
//
// If the ConnPrepareFunc returns an error, then it will be called the next time
// the connection is about to be used. Once ConnPrepareFunc returns nil for a
// given connection, it will not be called on that connection again.
type ConnPrepareFunc func(conn *sqlite.Conn) error
