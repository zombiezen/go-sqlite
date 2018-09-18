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

package sqlite_test

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"crawshaw.io/sqlite"
)

const (
	extCode = `
#include "sqlite3ext.h"
SQLITE_EXTENSION_INIT1

#include <stdlib.h>

static void hellofunc(
	sqlite3_context *context,
	int argc,
	sqlite3_value **argv){
		(void)argc;
		(void)argv;
		sqlite3_result_text(context, "Hello, World!", -1, SQLITE_STATIC);
	}

#ifdef _WIN32
__declspec(dllexport)
#endif
int sqlite3_hello_init(
	sqlite3 *db,
	char **pzErrMsg,
	const sqlite3_api_routines *pApi
){
		int rc = SQLITE_OK;
		SQLITE_EXTENSION_INIT2(pApi);
		(void)pzErrMsg;  /* Unused parameter */
		return sqlite3_create_function(db, "hello", 0, SQLITE_UTF8, 0, hellofunc, 0, 0);
}`
)

func TestLoadExtension(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	fout, err := os.Create(filepath.Join(tmpdir, "ext.c"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(fout, strings.NewReader(extCode)); err != nil {
		t.Fatal(err)
	}
	if err = fout.Close(); err != nil {
		t.Error(err)
	}
	include, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	switch runtime.GOOS {
	// See https://www.sqlite.org/loadext.html#build
	case "darwin":
		args = []string{"gcc", "-g", "-fPIC", "-I" + include, "-dynamiclib", "ext.c", "-o", "libhello.dylib"}
	case "linux":
		args = []string{"gcc", "-g", "-fPIC", "-I" + include, "-shared", "ext.c", "-o", "libhello.so"}
	case "windows":
		// TODO: add windows support
		fallthrough
	default:
		t.Skipf("unsupported OS: %s", runtime.GOOS)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = tmpdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("no gcc support: %s, %s", string(out), err)
	}
	c, err := sqlite.OpenConn(":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := c.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	libPath := filepath.Join(tmpdir, args[len(args)-1])
	err = c.LoadExtension(libPath, "")
	if err == nil {
		t.Error("loaded extension without enabling load extension")
	}
	err = c.EnableLoadExtension(true)
	if err != nil {
		t.Fatal(err)
	}
	err = c.LoadExtension(libPath, "")
	if err != nil {
		t.Fatal(err)
	}
	stmt := c.Prep("SELECT hello();")
	if _, err := stmt.Step(); err != nil {
		t.Fatal(err)
	}
	if got, want := stmt.ColumnText(0), "Hello, World!"; got != want {
		t.Errorf("failed to load extension, got: %s, want: %s", got, want)
	}
}
