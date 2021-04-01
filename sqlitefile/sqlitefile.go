// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Package sqlitefile provides functions for executing SQLite statements from a file.
package sqlitefile

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/fs"
	"zombiezen.com/go/sqlite/sqlitex"
)

// ExecOptions is the set of optional arguments for the functions in this package.
type ExecOptions struct {
	// Args is the set of positional arguments to bind to the statement. The first
	// element in the slice is ?1. See https://sqlite.org/lang_expr.html for more
	// details.
	Args []interface{}
	// Named is the set of named arguments to bind to the statement. Keys must
	// start with ':', '@', or '$'. See https://sqlite.org/lang_expr.html for more
	// details.
	Named map[string]interface{}
	// ResultFunc is called for each result row. If ResultFunc returns an error
	// then iteration ceases and Exec returns the error value.
	ResultFunc func(stmt *sqlite.Stmt) error
}

// Exec executes the single statement in the given SQL file.
// Exec is implemented using Conn.Prepare, so subsequent calls to Exec with the
// same statement will reuse the cached statement object.
func Exec(conn *sqlite.Conn, fsys fs.FS, filename string, opts *ExecOptions) error {
	query, err := readString(fsys, filename)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	stmt, err := conn.Prepare(strings.TrimSpace(query))
	if err != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	err = exec(stmt, opts)
	resetErr := stmt.Reset()
	if err != nil {
		// Don't strip the error query: we already do this inside exec.
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	if resetErr != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	return nil
}

// ExecTransient executes the single statement in the given SQL file without
// caching the underlying query.
func ExecTransient(conn *sqlite.Conn, fsys fs.FS, filename string, opts *ExecOptions) error {
	query, err := readString(fsys, filename)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	stmt, _, err := conn.PrepareTransient(strings.TrimSpace(query))
	if err != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	defer stmt.Finalize()
	err = exec(stmt, opts)
	resetErr := stmt.Reset()
	if err != nil {
		// Don't strip the error query: we already do this inside exec.
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	if resetErr != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	return nil
}

// PrepareTransient prepares an SQL statement from a file that is not cached by
// the Conn. Subsequent calls with the same query will create new Stmts.
// The caller is responsible for calling Finalize on the returned Stmt when the
// Stmt is no longer needed.
func PrepareTransient(conn *sqlite.Conn, fsys fs.FS, filename string) (*sqlite.Stmt, error) {
	query, err := readString(fsys, filename)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	stmt, _, err := conn.PrepareTransient(strings.TrimSpace(query))
	if err != nil {
		return nil, fmt.Errorf("prepare %s: %w", filename, err)
	}
	return stmt, nil
}

// ExecScript executes a script of SQL statements from a file.
//
// The script is wrapped in a SAVEPOINT transaction, which is rolled back on
// any error.
func ExecScript(conn *sqlite.Conn, fsys fs.FS, filename string, opts *ExecOptions) (err error) {
	queries, err := readString(fsys, filename)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	defer sqlitex.Save(conn)(&err)
	for {
		queries = strings.TrimSpace(queries)
		if queries == "" {
			return nil
		}
		stmt, trailingBytes, err := conn.PrepareTransient(queries)
		if err != nil {
			return fmt.Errorf("exec %s: %w", filename, err)
		}
		usedBytes := len(queries) - trailingBytes
		queries = queries[usedBytes:]
		err = exec(stmt, opts)
		stmt.Finalize()
		if err != nil {
			return fmt.Errorf("exec %s: %w", filename, err)
		}
	}
}

func exec(stmt *sqlite.Stmt, opts *ExecOptions) (err error) {
	if opts != nil {
		for i, arg := range opts.Args {
			setArg(stmt, i+1, reflect.ValueOf(arg))
		}
		if err := setNamed(stmt, opts.Named); err != nil {
			return err
		}
	}
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return err
		}
		if !hasRow {
			break
		}
		if opts != nil && opts.ResultFunc != nil {
			if err := opts.ResultFunc(stmt); err != nil {
				return err
			}
		}
	}
	return nil
}

func setArg(stmt *sqlite.Stmt, i int, v reflect.Value) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		stmt.BindInt64(i, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		stmt.BindInt64(i, int64(v.Uint()))
	case reflect.Float32, reflect.Float64:
		stmt.BindFloat(i, v.Float())
	case reflect.String:
		stmt.BindText(i, v.String())
	case reflect.Bool:
		stmt.BindBool(i, v.Bool())
	case reflect.Invalid:
		stmt.BindNull(i)
	default:
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			stmt.BindBytes(i, v.Bytes())
		} else {
			stmt.BindText(i, fmt.Sprint(v.Interface()))
		}
	}
}

func setNamed(stmt *sqlite.Stmt, args map[string]interface{}) error {
	if len(args) == 0 {
		return nil
	}
	for i, count := 1, stmt.BindParamCount(); i <= count; i++ {
		name := stmt.BindParamName(i)
		if name == "" {
			continue
		}
		arg, present := args[name]
		if !present {
			return fmt.Errorf("missing parameter %s", name)
		}
		setArg(stmt, i, reflect.ValueOf(arg))
	}
	return nil
}

func readString(fsys fs.FS, filename string) (string, error) {
	f, err := fsys.Open(filename)
	if err != nil {
		return "", err
	}
	content := new(strings.Builder)
	_, err = io.Copy(content, f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %w", filename, err)
	}
	return content.String(), nil
}
