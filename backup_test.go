// Copyright 2023 Ross Light
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestBackup(t *testing.T) {
	for _, isFull := range []bool{false, true} {
		name := "Full"
		if !isFull {
			name = "Incremental"
		}

		t.Run(name, func(t *testing.T) {
			src, err := sqlite.OpenConn(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			err = sqlitex.ExecuteTransient(src, `CREATE TABLE foo (x INTEGER PRIMARY KEY NOT NULL);`, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = sqlitex.ExecuteTransient(src, `INSERT INTO foo VALUES (1), (2), (3), (42);`, nil)
			if err != nil {
				t.Fatal(err)
			}

			dst, err := sqlite.OpenConn(":memory:")
			if err != nil {
				t.Fatal(err)
			}

			backup, err := sqlite.NewBackup(dst, "", src, "")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := backup.Close(); err != nil {
					t.Error(err)
				}
			}()

			if isFull {
				more, err := backup.Step(-1)
				if more || err != nil {
					t.Errorf("backup.Step(-1) = %t, %v; want false, <nil>", more, err)
				}
			} else {
				for {
					more, err := backup.Step(1)
					t.Logf("after Step: Remaining() = %d, PageCount() = %d", backup.Remaining(), backup.PageCount())
					if !more {
						if err != nil {
							t.Error("backup.Step(1):", err)
						}
						break
					}
				}
			}

			var got []int
			err = sqlitex.ExecuteTransient(dst, `SELECT x FROM foo ORDER BY x;`, &sqlitex.ExecOptions{
				ResultFunc: func(stmt *sqlite.Stmt) error {
					got = append(got, stmt.ColumnInt(0))
					return nil
				},
			})
			if err != nil {
				t.Error(err)
			}
			want := []int{1, 2, 3, 42}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("foo (-want +got):\n%s", diff)
			}
		})
	}
}
