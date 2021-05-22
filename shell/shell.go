// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

// Package shell provides a minimal SQLite REPL, similar to the built-in one.
// This is useful for providing a REPL with custom functions.
package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
	"modernc.org/libc"
	lib "modernc.org/sqlite/lib"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const (
	prompt             = "sqlite> "
	continuationPrompt = "   ...> "
)

// Run runs an interactive shell on the process's standard I/O.
func Run(conn *sqlite.Conn) {
	tls := libc.NewTLS()
	defer tls.Close()

	rl, err := readline.NewEx(&readline.Config{
		Prompt: prompt,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rl.Close()

	if readline.DefaultIsTerminal() {
		fmt.Printf("SQLite version %s\n", sqlite.Version)
	}
	var sql string
	for {
		if len(sql) > 0 {
			rl.SetPrompt(continuationPrompt)
		} else {
			rl.SetPrompt(prompt)
		}
		line, err := rl.Readline()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if len(sql) == 0 && strings.HasPrefix(line, ".") {
			wordEnd := strings.IndexFunc(line, unicode.IsSpace)
			if wordEnd == -1 {
				wordEnd = len(line)
			}
			switch word := line[1:wordEnd]; word {
			case "schema":
				err := sqlitex.ExecTransient(conn, `SELECT sql FROM sqlite_master;`, func(stmt *sqlite.Stmt) error {
					fmt.Println(stmt.ColumnText(0) + ";")
					return nil
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			case "quit":
				return
			default:
				fmt.Fprintf(os.Stderr, "unknown command .%s\n", word)
			}
			continue
		}
		line = strings.TrimLeftFunc(line, unicode.IsSpace)
		if line == "" {
			continue
		}
		sql += line + "\n"
		if !strings.Contains(line, ";") {
			continue
		}
		for isCompleteStmt(tls, sql) {
			sql = strings.TrimLeftFunc(sql, unicode.IsSpace)
			if sql == "" {
				break
			}
			stmt, trailingBytes, err := conn.PrepareTransient(sql)
			sql = sql[len(sql)-trailingBytes:]
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			for {
				hasData, err := stmt.Step()
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					break
				}
				if !hasData {
					break
				}
				row := new(strings.Builder)
				for i, n := 0, stmt.ColumnCount(); i < n; i++ {
					if i > 0 {
						row.WriteString("|")
					}
					switch stmt.ColumnType(i) {
					case sqlite.TypeInteger:
						fmt.Fprint(row, stmt.ColumnInt64(i))
					case sqlite.TypeFloat:
						fmt.Fprint(row, stmt.ColumnFloat(i))
					case sqlite.TypeBlob:
						buf := make([]byte, stmt.ColumnLen(i))
						stmt.ColumnBytes(i, buf)
						row.Write(buf)
					case sqlite.TypeText:
						row.WriteString(stmt.ColumnText(i))
					}
				}
				fmt.Println(row)
			}
			stmt.Finalize()
		}
	}
}

func isCompleteStmt(tls *libc.TLS, s string) bool {
	if s == "" {
		return true
	}
	c, err := libc.CString(s + ";")
	if err != nil {
		panic(err)
	}
	defer libc.Xfree(tls, c)
	return lib.Xsqlite3_complete(tls, c) != 0
}
