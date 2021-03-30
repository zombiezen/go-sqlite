// Copyright (c) 2018 David Crawshaw <david@zentus.com>
// Copyright (c) 2021 Ross Light <ross@zombiezen.com>
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
//
// SPDX-License-Identifier: ISC

package sqlite_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"reflect"
	"sync"
	"testing"
	"time"

	"zombiezen.com/go/sqlite"
)

var _ interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
} = (*sqlite.Blob)(nil)

func TestBlob(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", 5)
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}
	rowid := c.LastInsertRowID()
	t.Logf("blobs rowid: %d", rowid)

	b, err := c.OpenBlob("", "blobs", "col", rowid, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	n, err := b.Write([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("b.WriteAt n=%d, want 3", n)
	}
	if _, err := b.Seek(1, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	n, err = b.Write([]byte{2, 3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("b.WriteAt n=%d, want 4", n)
	}
	if size := b.Size(); size != 5 {
		t.Fatalf("b.Size=%d, want 5", size)
	}
	if _, err := b.Seek(1, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	n, err = b.Write([]byte{2, 3, 4, 5, 6}) // too long
	if err == nil {
		t.Fatalf("WriteAt too long, but no error")
	}
	if err := b.Close(); err != nil {
		t.Error(err)
	}

	b, err = c.OpenBlob("", "blobs", "col", rowid, false)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 5)
	n, err = io.ReadFull(b, got)
	if n != len(got) {
		t.Fatalf("b.Read=%d, want len(got)=%d", n, len(got))
	}
	want := []byte{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("b.Read got %v, want %v", got, want)
	}
	if _, err := b.Seek(3, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	n, err = io.ReadFull(b, got[3:])
	if n != len(got)-3 {
		t.Fatalf("b.Read(got, 3)=%d, want len(got)-3=%d", n, len(got)-3)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("b.Read(got, 3) %v, want %v", got, want)
	}
}

func TestConcurrentBlobSpins(t *testing.T) {
	t.Skip("TODO(soon)")

	flags := sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenURI | sqlite.OpenNoMutex | sqlite.OpenSharedCache
	c, err := sqlite.OpenConn("file::memory:?mode=memory", flags)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := sqlite.OpenConn("file::memory:?mode=memory", flags)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer c2.Close()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", 1024)
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	blobRow1 := c.LastInsertRowID()
	if err := stmt.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	blobRow2 := c.LastInsertRowID()
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}

	blob1, err := c.OpenBlob("", "blobs", "col", blobRow1, true)
	if err != nil {
		t.Errorf("OpenBlob: %v", err)
		return
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		blob1.Close()
	}()

	// countBefore := sqlite.ConnCount(c2)
	blob2, err := c2.OpenBlob("", "blobs", "col", blobRow2, true)
	// countAfter := sqlite.ConnCount(c2)
	if err != nil {
		t.Errorf("OpenBlob: %v", err)
		return
	}
	blob2.Close()

	// TODO(maybe)
	// if spins := countAfter - countBefore - 1; spins > 1 {
	// 	t.Errorf("expecting no more than 1 spin, got %d", spins)
	// }
}

// TestConcurrentBlobWrites looks for unexpected SQLITE_LOCKED errors
// when using the (default) shared cache.
func TestConcurrentBlobWrites(t *testing.T) {
	t.Skip("TODO(soon)")

	flags := sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenURI | sqlite.OpenNoMutex | sqlite.OpenSharedCache

	const numBlobs = 5
	c, err := sqlite.OpenConn("file::memory:?mode=memory", flags)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", 1024)
	var blobRowIDs []int64
	for i := 0; i < numBlobs; i++ {
		if _, err := stmt.Step(); err != nil {
			t.Fatal(err)
		}
		blobRowIDs = append(blobRowIDs, c.LastInsertRowID())
		if err := stmt.Reset(); err != nil {
			t.Fatal(err)
		}
	}
	if err := stmt.Finalize(); err != nil {
		t.Error(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < numBlobs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b := make([]byte, 1024)
			b[0] = byte(i)

			c, err := sqlite.OpenConn("file::memory:?mode=memory", flags)
			if err != nil {
				t.Error(err)
				return
			}
			defer c.Close()

			blob, err := c.OpenBlob("", "blobs", "col", blobRowIDs[i], true)
			if err != nil {
				t.Errorf("OpenBlob: %v (i=%d)", err, i)
				return
			}
			defer blob.Close()
			for j := 0; j < 10; j++ {
				b[1] = byte(j)

				if _, err := blob.Seek(0, io.SeekStart); err != nil {
					t.Error(err)
					return
				}
				n, err := blob.Write(b)
				if err != nil {
					t.Errorf("Blob.WriteAt: %v (i=%d, j=%d)", err, i, j)
					return
				}
				if n != len(b) {
					t.Errorf("n=%d, want %d (i=%d, j=%d)", n, len(b), i, j)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestBlobClose(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", 5)
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	rowid := c.LastInsertRowID()
	t.Logf("blobs rowid: %d", rowid)

	b, err := c.OpenBlob("", "blobs", "col", rowid, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err == nil {
		t.Error("no error on second close")
	}
	if _, err := b.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte{1}); err == nil {
		t.Error("want error on write-after-close")
	}
	if _, err := b.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadFull(b, make([]byte, 1)); err == nil {
		t.Error("want error on read-after-close")
	}
}

func TestBlobReadWrite(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", 5)
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	rowid := c.LastInsertRowID()
	t.Logf("blobs rowid: %d", rowid)

	b, err := c.OpenBlob("", "blobs", "col", rowid, true)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if _, err := b.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte{4, 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte{6}); err == nil {
		t.Error("Write past end of blob: want error; got <nil>")
	}
	if _, err := b.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if got, err := ioutil.ReadAll(b); err != nil {
		t.Fatal(err)
	} else if want := []byte{1, 2, 3, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// See https://github.com/golang/go/issues/28606
func TestBlobPtrs(t *testing.T) {
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	if _, err := c.Prep("DROP TABLE IF EXISTS blobs;").Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prep("CREATE TABLE blobs (col BLOB);").Step(); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	gzw := gzip.NewWriter(buf)
	gzw.Write([]byte("hello"))
	gzw.Close()
	n := buf.Len()

	stmt := c.Prep("INSERT INTO blobs (col) VALUES ($col);")
	stmt.SetZeroBlob("$col", int64(n))
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	rowid := c.LastInsertRowID()
	t.Logf("blobs rowid: %d", rowid)

	blob, err := c.OpenBlob("", "blobs", "col", rowid, true)
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()

	gzw = gzip.NewWriter(blob)
	gzw.Write([]byte("hello"))
	gzw.Close()

	blob.Seek(0, 0)

	gzr, err := gzip.NewReader(blob)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(gzr)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "hello" {
		t.Errorf("read %q, want %q", got, "hello")
	}
	if err := gzr.Close(); err != nil {
		t.Fatal(err)
	}
}
