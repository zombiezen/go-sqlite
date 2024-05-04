// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package sqlitemigration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestPool(t *testing.T) {
	ctx := context.Background()

	t.Run("NoMigrations", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
		}
		state := new(eventRecorder)
		pool := NewPool(filepath.Join(t.TempDir(), "no-migrations.db"), schema, Options{
			Flags:          sqlite.OpenReadWrite | sqlite.OpenCreate,
			OnStartMigrate: state.startMigrateFunc(),
			OnReady:        state.readyFunc(),
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
			if err := pool.CheckHealth(); err == nil {
				t.Error("after Close, CheckHealth() = <nil>; want error")
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.migrationStarted != 1 {
			t.Errorf("OnStartMigrate called %d times; want 1", state.migrationStarted)
		}
		if state.ready != 1 {
			t.Errorf("OnReady called %d times; want 1", state.ready)
		}
		if err := pool.CheckHealth(); err != nil {
			t.Errorf("after successful Get, CheckHealth = %v; want <nil>", err)
		}
		called := false
		err = sqlitex.ExecTransient(conn, "PRAGMA application_id;", func(stmt *sqlite.Stmt) error {
			called = true
			if got, want := stmt.ColumnInt32(0), int32(0xedbeef); got != want {
				t.Errorf("application_id = %#x; want %#x", got, want)
			}
			return nil
		})
		if err != nil {
			t.Errorf("PRAGMA application_id: %v", err)
		} else if !called {
			t.Error("PRAGMA application_id not called")
		}
		pool.Put(conn)
	})

	t.Run("DoesNotMigrateDifferentDatabase", func(t *testing.T) {
		// Create another.db with a single table.
		// Don't set application ID.
		dir := t.TempDir()
		err := withTestConn(dir, "another.db", func(conn *sqlite.Conn) error {
			err := sqlitex.ExecTransient(conn, `create table foo ( id integer primary key not null );`, nil)
			if err != nil {
				return fmt.Errorf("create table: %v", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// Try to open the pool.
		schema := Schema{
			AppID: 0xedbeef,
		}
		state := new(eventRecorder)
		pool := NewPool(filepath.Join(dir, "another.db"), schema, Options{
			Flags:          sqlite.OpenReadWrite | sqlite.OpenCreate,
			OnStartMigrate: state.startMigrateFunc(),
			OnReady:        state.readyFunc(),
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
			if err := pool.CheckHealth(); err == nil {
				t.Error("after Close, CheckHealth() = <nil>; want error")
			}
		}()
		conn, err := pool.Get(ctx)
		t.Logf("pool.Get error: %v", err)
		if err == nil {
			pool.Put(conn)
			return
		}
		if state.migrationStarted != 0 {
			t.Errorf("OnStartMigrate called %d times; want 0", state.migrationStarted)
		}
		if state.ready != 0 {
			t.Errorf("OnReady called %d times; want 0", state.ready)
		}
		if err := pool.CheckHealth(); err == nil {
			t.Errorf("CheckHealth = <nil>; want error")
		}

		// Verify that application ID is not set.
		err = withTestConn(dir, "another.db", func(conn *sqlite.Conn) error {
			called := false
			err = sqlitex.ExecTransient(conn, "PRAGMA application_id;", func(stmt *sqlite.Stmt) error {
				called = true
				if got, want := stmt.ColumnInt32(0), int32(0); got != want {
					t.Errorf("application_id = %#x; want %#x", got, want)
				}
				return nil
			})
			if err != nil {
				t.Errorf("PRAGMA application_id: %v", err)
			} else if !called {
				t.Error("PRAGMA application_id not called")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ZeroAppID", func(t *testing.T) {
		const dbName = "zeroid.db"
		schema1 := Schema{
			AppID: 0,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, dbName), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, dbName), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if got != 42 {
			t.Errorf("select id = %d; want 42", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("OneMigration", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		state := new(eventRecorder)
		pool := NewPool(filepath.Join(t.TempDir(), "one-migration.db"), schema, Options{
			Flags:          sqlite.OpenReadWrite | sqlite.OpenCreate,
			OnStartMigrate: state.startMigrateFunc(),
			OnReady:        state.readyFunc(),
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
			if err := pool.CheckHealth(); err == nil {
				t.Error("after Close, CheckHealth() = <nil>; want error")
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		if state.migrationStarted != 1 {
			t.Errorf("OnStartMigrate called %d times; want 1", state.migrationStarted)
		}
		if state.ready != 1 {
			t.Errorf("OnReady called %d times; want 1", state.ready)
		}
		if err := pool.CheckHealth(); err != nil {
			t.Errorf("after successful Get, CheckHealth = %v; want <nil>", err)
		}
		err = sqlitex.ExecTransient(conn, "insert into foo values (42);", nil)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("TwoMigrations", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
		}
		state := new(eventRecorder)
		pool := NewPool(filepath.Join(t.TempDir(), "two-migrations.db"), schema, Options{
			Flags:          sqlite.OpenReadWrite | sqlite.OpenCreate,
			OnStartMigrate: state.startMigrateFunc(),
			OnReady:        state.readyFunc(),
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		if state.migrationStarted != 1 {
			t.Errorf("OnStartMigrate called %d times; want 1", state.migrationStarted)
		}
		if state.ready != 1 {
			t.Errorf("OnReady called %d times; want 1", state.ready)
		}
		if err := pool.CheckHealth(); err != nil {
			t.Errorf("after successful Get, CheckHealth = %v; want <nil>", err)
		}
		var got int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if got != 42 {
			t.Errorf("select id = %d; want 42", got)
		}
	})

	t.Run("PartialMigration", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null ); insert into foo values (1);`,
				`insert into foo values (42); insert into bar values (57);`,
			},
		}
		state := new(eventRecorder)
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "partial-migration.db"), schema, Options{
			Flags:          sqlite.OpenReadWrite | sqlite.OpenCreate,
			OnStartMigrate: state.startMigrateFunc(),
			OnReady:        state.readyFunc(),
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
			if err := pool.CheckHealth(); err == nil {
				t.Error("after Close, CheckHealth() = <nil>; want error")
			}
		}()
		conn, err := pool.Get(ctx)
		t.Logf("pool.Get error: %v", err)
		if err == nil {
			pool.Put(conn)
			return
		}
		if state.migrationStarted != 1 {
			t.Errorf("OnStartMigrate called %d times; want 1", state.migrationStarted)
		}
		if state.ready != 0 {
			t.Errorf("OnReady called %d times; want 0", state.ready)
		}
		if err := pool.CheckHealth(); err == nil {
			t.Error("CheckHealth() = <nil>; want error")
		}

		// Verify that the first migration is applied and that none of the second
		// migration is applied.
		err = withTestConn(dir, "partial-migration.db", func(conn *sqlite.Conn) error {
			var got int
			err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
				got = stmt.ColumnInt(0)
				return nil
			})
			if err != nil {
				return err
			}
			if got != 1 {
				t.Errorf("select id = %d; want 1", got)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("MigrationsDontRepeat", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "migrations-dont-repeat.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		err = sqlitex.ExecTransient(conn, "insert into foo values (42);", nil)
		if err != nil {
			t.Error(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "migrations-dont-repeat.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		err = sqlitex.ExecTransient(conn, "insert into foo values (56);", nil)
		if err != nil {
			t.Error(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("IncrementalMigration", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "incremental-migration.db"), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "incremental-migration.db"), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if got != 42 {
			t.Errorf("select id = %d; want 42", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("Repeatable/IncrementalMigration", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
			RepeatableMigration: `insert into foo values (333);`,
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "repeatable-incremental.db"), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "repeatable-incremental.db"), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got []int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = append(got, stmt.ColumnInt(0))
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if !cmp.Equal(got, []int{42, 333}) {
			t.Errorf("select id = %v; want [42 333]", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("Repeatable/IncrementalMigrationEmpty", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				"",
			},
			RepeatableMigration: `insert into foo values (333);`,
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "repeatable-incremental.db"), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "repeatable-incremental.db"), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got []int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = append(got, stmt.ColumnInt(0))
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if !cmp.Equal(got, []int{333}) {
			t.Errorf("select id = %v; want [333]", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("Repeatable/IncrementalMigrationFailure", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
			RepeatableMigration: `insert into bar values (333);`,
		}

		// Run 1
		dbPath := filepath.Join(t.TempDir(), "repeatable-fail.db")
		pool := NewPool(dbPath, schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(dbPath, schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err == nil {
			pool.Put(conn)
			t.Error("Second migration did not fail")
		}
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		err = withTestConn(filepath.Dir(dbPath), filepath.Base(dbPath), func(conn *sqlite.Conn) error {
			var got []int
			err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
				got = append(got, stmt.ColumnInt(0))
				return nil
			})
			if err != nil {
				return err
			}
			if len(got) != 0 {
				return fmt.Errorf("select id = %v; want []", got)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Repeatable/SameVersion", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
			RepeatableMigration: `insert into foo values (333);`,
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "repeatable-sameversion.db"), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "repeatable-sameversion.db"), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got []int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = append(got, stmt.ColumnInt(0))
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if len(got) > 0 {
			t.Errorf("select id = %v; want []", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("FutureVersion", func(t *testing.T) {
		schema1 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
				`insert into foo values (42);`,
			},
		}
		schema2 := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}

		// Run 1
		dir := t.TempDir()
		pool := NewPool(filepath.Join(dir, "future-version.db"), schema1, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err := pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}

		// Run 2
		pool = NewPool(filepath.Join(dir, "future-version.db"), schema2, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
		})
		conn, err = pool.Get(ctx)
		if err != nil {
			pool.Close()
			t.Fatal(err)
		}
		var got int
		err = sqlitex.ExecTransient(conn, "select id from foo order by id;", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Error(err)
		} else if got != 42 {
			t.Errorf("select id = %d; want 42", got)
		}
		pool.Put(conn)
		if err := pool.Close(); err != nil {
			t.Error("pool.Close:", err)
		}
	})

	t.Run("CustomFunctionInMigration", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );
				insert into foo (id) values (theAnswer());`,
			},
		}
		pool := NewPool(filepath.Join(t.TempDir(), "custom-schema-function.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
			PrepareConn: func(conn *sqlite.Conn) error {
				return conn.CreateFunction("theAnswer", &sqlite.FunctionImpl{
					NArgs:         0,
					Deterministic: true,
					Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
						return sqlite.IntegerValue(42), nil
					},
				})
			},
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		var got int
		err = sqlitex.ExecTransient(conn, "select id from foo limit 1;", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != 42 {
			t.Errorf("got %d; want 42", got)
		}
	})

	t.Run("CustomFunctionInGet", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
		}
		pool := NewPool(filepath.Join(t.TempDir(), "custom-get-function.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
			PrepareConn: func(conn *sqlite.Conn) error {
				return conn.CreateFunction("theAnswer", &sqlite.FunctionImpl{
					NArgs:         0,
					Deterministic: true,
					Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
						return sqlite.IntegerValue(42), nil
					},
				})
			},
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		var got int
		err = sqlitex.ExecTransient(conn, "select theAnswer();", func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != 42 {
			t.Errorf("got %d; want 42", got)
		}
	})

	t.Run("DisableForeignKeys", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( foreign_keys_enabled bool ); insert into foo values ((select * from pragma_foreign_keys()));`,
			},
			MigrationOptions: []*MigrationOptions{
				{DisableForeignKeys: true},
			},
		}
		pool := NewPool(filepath.Join(t.TempDir(), "disable-foreign-keys.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
			PrepareConn: func(conn *sqlite.Conn) error {
				return sqlitex.ExecTransient(conn, "PRAGMA foreign_keys = on;", nil)
			},
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		duringMigration, err := sqlitex.ResultBool(conn.Prep("select foreign_keys_enabled from foo;"))
		if err != nil {
			t.Error(err)
		} else if duringMigration {
			t.Error("Foreign keys were enabled during migration")
		}
		afterMigration, err := sqlitex.ResultBool(conn.Prep("PRAGMA foreign_keys;"))
		if err != nil {
			t.Error(err)
		} else if !afterMigration {
			t.Error("Foreign keys were left disabled after migration")
		}
	})

	t.Run("NoTouchForeignKeys", func(t *testing.T) {
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( foreign_keys_enabled bool ); insert into foo values ((select * from pragma_foreign_keys()));`,
			},
		}
		pool := NewPool(filepath.Join(t.TempDir(), "no-touch-foreign-keys.db"), schema, Options{
			Flags: sqlite.OpenReadWrite | sqlite.OpenCreate,
			PrepareConn: func(conn *sqlite.Conn) error {
				return sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = on;", nil)
			},
		})
		defer func() {
			if err := pool.Close(); err != nil {
				t.Error("pool.Close:", err)
			}
		}()
		conn, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Put(conn)
		duringMigration, err := sqlitex.ResultBool(conn.Prep("select foreign_keys_enabled from foo;"))
		if err != nil {
			t.Error(err)
		} else if !duringMigration {
			t.Error("Foreign keys were disabled during migration")
		}
		afterMigration, err := sqlitex.ResultBool(conn.Prep("PRAGMA foreign_keys;"))
		if err != nil {
			t.Error(err)
		} else if !afterMigration {
			t.Error("Foreign keys were disabled after migration")
		}
	})
}

func TestMigrate(t *testing.T) {
	t.Run("NoTouchForeignKeys", func(t *testing.T) {
		conn, err := sqlite.OpenConn(filepath.Join(t.TempDir(), "no-touch-foreign-keys.db"), sqlite.OpenReadWrite, sqlite.OpenCreate)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		err = sqlitex.ExecuteTransient(conn, `PRAGMA foreign_keys = on;`, nil)
		if err != nil {
			t.Fatal(err)
		}

		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( foreign_keys_enabled bool ); insert into foo values ((select * from pragma_foreign_keys()));`,
			},
		}
		if err := Migrate(context.Background(), conn, schema); err != nil {
			t.Error(err)
		}
		duringMigration, err := sqlitex.ResultBool(conn.Prep("select foreign_keys_enabled from foo;"))
		if err != nil {
			t.Error(err)
		} else if !duringMigration {
			t.Error("Foreign keys were disabled during migration")
		}
		afterMigration, err := sqlitex.ResultBool(conn.Prep("PRAGMA foreign_keys;"))
		if err != nil {
			t.Error(err)
		} else if !afterMigration {
			t.Error("Foreign keys were disabled after migration")
		}
	})

	t.Run("Concurrent", func(t *testing.T) {
		ctx := context.Background()
		dbPath := filepath.Join(t.TempDir(), "concurrent.db")
		schema := Schema{
			AppID: 0xedbeef,
			Migrations: []string{
				`create table foo ( id integer primary key not null );`,
			},
		}

		// Attempt to perform migrations while writing.
		var wg sync.WaitGroup
		defer wg.Wait()
		const numConcurrent = 5
		wg.Add(numConcurrent)
		for i := 0; i < numConcurrent; i++ {
			go func(i int) {
				defer wg.Done()

				conn, err := sqlite.OpenConn(dbPath, sqlite.OpenReadWrite, sqlite.OpenCreate)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() {
					if err := conn.Close(); err != nil {
						t.Error(err)
					}
				}()
				if err := Migrate(ctx, conn, schema); err != nil {
					t.Error("Migrate:", err)
				}
			}(i)
		}

		// Migrate and issue writes on one connection.
		conn, err := sqlite.OpenConn(dbPath, sqlite.OpenReadWrite, sqlite.OpenCreate)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		}()
		if err := Migrate(ctx, conn, schema); err != nil {
			t.Fatal("Migrate:", err)
		}
		for i := 0; i < 150; i++ {
			if err := sqlitex.Execute(conn, "insert into foo values (?)", &sqlitex.ExecOptions{
				Args: []any{i},
			}); err != nil {
				t.Error("insert query:", err)
			}
		}
	})
}

// withTestConn makes an independent connection to the given database.
func withTestConn(dir, name string, f func(*sqlite.Conn) error) error {
	conn, err := sqlite.OpenConn(filepath.Join(dir, name), sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := f(conn); err != nil {
		return err
	}
	return nil
}

type eventRecorder struct {
	migrationStarted int
	ready            int
}

func (rec *eventRecorder) startMigrateFunc() SignalFunc {
	return func() {
		rec.migrationStarted++
	}
}

func (rec *eventRecorder) readyFunc() SignalFunc {
	return func() {
		rec.ready++
	}
}
