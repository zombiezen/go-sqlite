// Copyright 2024 Roxy Light
// SPDX-License-Identifier: ISC

package sqlitex

import (
	"testing"

	"zombiezen.com/go/sqlite"
)

func TestResultInt64(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = ExecuteScript(conn, `
CREATE TABLE foo (
	id integer not null primary key
);

INSERT INTO foo VALUES (1), (2);`, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Single", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT 42;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		const want = 42
		got, err := ResultInt64(stmt)
		if got != want || err != nil {
			t.Errorf("ResultInt64(...) = %d, %v; want %d, <nil>", got, err, want)
		}
	})

	t.Run("Multiple", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id FROM foo;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		n, err := ResultInt64(stmt)
		if n != 0 || err == nil {
			t.Errorf("ResultInt64(...) = %d, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})

	t.Run("NoRows", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id FROM foo WHERE id > 3;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		n, err := ResultInt64(stmt)
		if n != 0 || err == nil {
			t.Errorf("ResultInt64(...) = %d, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})
}

func TestResultBool(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = ExecuteScript(conn, `
CREATE TABLE foo (
	id integer not null primary key
);

INSERT INTO foo VALUES (1), (2);`, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("False", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT false;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultBool(stmt)
		if got || err != nil {
			t.Errorf("ResultBool(...) = %t, %v; want false, <nil>", got, err)
		}
	})

	t.Run("True", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT true;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultBool(stmt)
		if !got || err != nil {
			t.Errorf("ResultBool(...) = %t, %v; want true, <nil>", got, err)
		}
	})

	t.Run("Multiple", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id = 1 FROM foo;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultBool(stmt)
		if got || err == nil {
			t.Errorf("ResultBool(...) = %t, %v; want false, <error>", got, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})

	t.Run("NoRows", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id = 1 FROM foo WHERE id > 3;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultBool(stmt)
		if got || err == nil {
			t.Errorf("ResultBool(...) = %t, %v; want false, <error>", got, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})
}

func TestResultText(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = ExecuteScript(conn, `
CREATE TABLE foo (
	id integer not null primary key,
	my_blob blob
);

INSERT INTO foo VALUES (1, CAST('hi' AS BLOB)), (2, CAST('bye' AS BLOB));`, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Single", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo WHERE id = 1;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		const want = "hi"
		got, err := ResultText(stmt)
		if got != want || err != nil {
			t.Errorf("ResultText(...) = %q, %v; want %q, <nil>", got, err, want)
		}
	})

	t.Run("Multiple", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultText(stmt)
		if got != "" || err == nil {
			t.Errorf("ResultText(...) = %q, %v; want 0, <error>", got, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})

	t.Run("NoRows", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo WHERE id = 3;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		got, err := ResultText(stmt)
		if got != "" || err == nil {
			t.Errorf("ResultText(...) = %q, %v; want 0, <error>", got, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})
}

func TestResultFloat(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = ExecuteScript(conn, `
CREATE TABLE foo (
	id integer not null primary key
);

INSERT INTO foo VALUES (1), (2);`, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Single", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT 42;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		const want = 42.0
		got, err := ResultFloat(stmt)
		if got != want || err != nil {
			t.Errorf("ResultFloat(...) = %g, %v; want %g, <nil>", got, err, want)
		}
	})

	t.Run("Multiple", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id FROM foo;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		n, err := ResultFloat(stmt)
		if n != 0 || err == nil {
			t.Errorf("ResultFloat(...) = %g, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})

	t.Run("NoRows", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT id FROM foo WHERE id > 3;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		n, err := ResultFloat(stmt)
		if n != 0 || err == nil {
			t.Errorf("ResultFloat(...) = %g, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})
}

func TestResultBytes(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = ExecuteScript(conn, `
CREATE TABLE foo (
	id integer not null primary key,
	my_blob blob
);

INSERT INTO foo VALUES (1, CAST('hi' AS BLOB)), (2, CAST('bye' AS BLOB));`, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Single", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo WHERE id = 1;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		const want = "hi"
		buf := make([]byte, 4096)
		n, err := ResultBytes(stmt, buf)
		if n != len(want) || err != nil {
			t.Errorf("ResultBytes(...) = %d, %v; want %d, <nil>", n, err, len(want))
		}
		if got := string(buf[:n]); got != want {
			t.Errorf("result = %q; want %q", got, want)
		}
	})

	t.Run("Multiple", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		buf := make([]byte, 4096)
		n, err := ResultBytes(stmt, buf)
		if n != 0 || err == nil {
			t.Errorf("ResultBytes(...) = %d, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})

	t.Run("NoRows", func(t *testing.T) {
		stmt, _, err := conn.PrepareTransient(`SELECT my_blob FROM foo WHERE id = 3;`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Finalize()

		buf := make([]byte, 4096)
		n, err := ResultBytes(stmt, buf)
		if n != 0 || err == nil {
			t.Errorf("ResultBytes(...) = %d, %v; want 0, <error>", n, err)
		} else {
			t.Log("Returned (expected) error:", err)
		}
	})
}
