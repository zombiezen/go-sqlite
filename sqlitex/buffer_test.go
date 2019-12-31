// Copyright (c) 2018 David Crawshaw <david@zentus.com>
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

package sqlitex

import (
	"bytes"
	"io"
	"testing"

	"crawshaw.io/iox/ioxtest"
	"crawshaw.io/sqlite"
)

func TestBuffer(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	data := make([]byte, 64*1024+1)
	for i := 0; i < 64; i++ {
		data[i*1024] = byte(i)
		data[i*1024+1023] = byte(i)
	}

	buf, err := NewBufferSize(conn, 1024)
	if err != nil {
		t.Fatal(err)
	}

	if n, err := buf.Write(data); err != nil {
		t.Fatal(err)
	} else if n != len(data) {
		t.Errorf("buf.Write(data) n=%d, want len(data)=%d", n, len(data))
	}

	if l := int(buf.Len()); l != len(data) {
		t.Errorf("buf.Len()=%d, want %d", l, len(data))
	}

	got1 := make([]byte, 1024)
	if n, err := buf.Read(got1); err != nil {
		t.Fatal(err)
	} else if n != len(got1) {
		t.Errorf("buf.Read(got1) n=%d, want len(got1)=%d", n, len(got1))
	}
	if !bytes.Equal(got1, data[:len(got1)]) {
		t.Errorf("got1 does not match, [0]=%d, [1]=%d, [1023]=%d", got1[0], got1[1], got1[1023])
	}

	if l := int(buf.Len()); l != len(data)-len(got1) {
		t.Errorf("buf.Len()=%d, want %d", l, len(data)-len(got1))
	}

	b := new(bytes.Buffer)
	b.Write(got1)
	if _, err := io.Copy(b, buf); err != nil {
		t.Errorf("io.Copy err=%v", err)
	}
	if !bytes.Equal(b.Bytes(), data) {
		t.Errorf("b.Bytes and data do not match")
	}

	buf.Reset()
	if _, err := buf.Write(got1); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if _, err := io.Copy(b, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), got1) {
		t.Errorf("b.Bytes and got1 do not match")
	}

	if err := buf.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentBuffer(t *testing.T) {
	// Make sure the shared cache table lock does not
	// apply to blob buffers (because we use temp tables).
	dbpool, err := Open("file::memory:?mode=memory", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	conn1 := dbpool.Get(nil)
	conn2 := dbpool.Get(nil)
	defer func() {
		dbpool.Put(conn1)
		dbpool.Put(conn2)
		dbpool.Close()
	}()

	b1a, err := NewBuffer(conn1)
	if err != nil {
		t.Fatal(err)
	}
	defer b1a.Close()

	b1b, err := NewBuffer(conn1)
	if err != nil {
		t.Fatal(err)
	}
	defer b1b.Close()

	b2, err := NewBuffer(conn2)
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()
}

func TestBufferRand(t *testing.T) {
	conn, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	buf, err := NewBufferSize(conn, 1<<18)
	if err != nil {
		t.Fatal(err)
	}

	ft := ioxtest.Tester{
		F1: buf,
		F2: new(bytes.Buffer),
		T:  t,
	}
	ft.Run()
	if err := buf.Close(); err != nil {
		t.Fatal(err)
	}
}
