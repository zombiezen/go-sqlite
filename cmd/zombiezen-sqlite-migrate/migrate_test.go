// Copyright 2021 Roxy Light
// SPDX-License-Identifier: ISC

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestProcess(t *testing.T) {
	crawshawModule, err := skeletonModule("crawshaw.io/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	bassModule, err := skeletonModule("zombiezen.com/go/bass")
	if err != nil {
		t.Fatal(err)
	}
	sqliteModule, err := skeletonModule("zombiezen.com/go/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	rootDir := filepath.Join("testdata", "TestProcess")
	testRunContents, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	const mainPkgPath = "example.com/foo"
	const mainFilename = "file.go"
	for _, ent := range testRunContents {
		name := ent.Name()
		if !ent.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(rootDir, name)
			want, err := os.ReadFile(filepath.Join(dir, "want.go"))
			if err != nil {
				t.Fatal(err)
			}
			want, err = format.Source(want)
			if err != nil {
				t.Fatal(err)
			}

			originalFile := filepath.Join(dir, "original.go")
			e := packagestest.Export(t, packagestest.Modules, []packagestest.Module{
				crawshawModule,
				bassModule,
				sqliteModule,
				{
					Name: mainPkgPath,
					Files: map[string]any{
						mainFilename: packagestest.Copy(originalFile),
					},
				},
			})
			cfg := new(packages.Config)
			*cfg = *e.Config
			cfg.Mode = processMode
			pkgs, err := packages.Load(cfg, "pattern="+mainPkgPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(pkgs) != 1 {
				t.Fatalf("Found %d packages; want 1", len(pkgs))
			}
			pkg := pkgs[0]
			if len(pkg.Errors) > 0 {
				for _, err := range pkg.Errors {
					t.Errorf("Load %s: %v", originalFile, err)
				}
				return
			}
			if len(pkg.Syntax) != 1 {
				t.Fatalf("Found %d parsed files; want 1", len(pkg.Syntax))
			}
			file := pkg.Syntax[0]

			for _, err := range process(pkg, file) {
				t.Logf("process: %v", err)
			}

			got, err := formatFile(new(bytes.Buffer), originalFile, pkg.Fset, file)
			if err != nil {
				t.Fatalf("Formatting output: %v", err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("diff a/%s b/%s:\n%s", mainFilename, mainFilename, diff)
			}
		})
	}
}

func skeletonModule(importPath string) (packagestest.Module, error) {
	mod := packagestest.Module{
		Name:  importPath,
		Files: make(map[string]any),
	}
	dir := filepath.Join("testdata", "skeleton", filepath.FromSlash(importPath))
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relpath := strings.TrimPrefix(path, dir+string(filepath.Separator))
		mod.Files[filepath.ToSlash(relpath)] = packagestest.Copy(path)
		return nil
	})
	if err != nil {
		return packagestest.Module{}, fmt.Errorf("load skeleton module %q: %w", importPath, err)
	}
	return mod, nil
}

type logWriter struct {
	logger interface{ Logf(string, ...any) }
	buf    []byte
}

func (lw *logWriter) Write(p []byte) (int, error) {
	lastLF := bytes.LastIndexByte(p, '\n')
	if lastLF == -1 {
		lw.buf = append(lw.buf, p...)
		return len(p), nil
	}
	if len(lw.buf) > 0 {
		lw.buf = append(lw.buf, p[:lastLF]...)
		lw.logger.Logf("%s", lw.buf)
		lw.buf = lw.buf[:0]
	} else {
		lw.logger.Logf("%s", p[:lastLF])
	}
	lw.buf = append(lw.buf, p[lastLF+1:]...)
	return len(p), nil
}
