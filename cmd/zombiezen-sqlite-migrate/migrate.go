// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"os/signal"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"zombiezen.com/go/bass/sigterm"
)

const programName = "zombiezen-sqlite-migrate"

func main() {
	writeFiles := flag.Bool("w", false, "write to file")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), sigterm.Signals()...)
	exitCode := run(ctx, *writeFiles, flag.Args())
	cancel()
	os.Exit(exitCode)
}

func run(ctx context.Context, writeFiles bool, patterns []string) int {
	cfg := &packages.Config{
		Context: ctx,
		Mode:    processMode,
		// TODO(soon): Tests: true,
	}
	fmt.Fprintf(os.Stderr, "%s: loading packages...\n", programName)
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", programName, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s: packages loaded\n", programName)

	buf := new(bytes.Buffer)
	var errorList []error
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			errorList = append(errorList, process(pkg, f)...)
			origPath := pkg.Fset.File(f.Pos()).Name()
			if writeFiles {
				if err := write(buf, origPath, pkg.Fset, f); err != nil {
					errorList = append(errorList, err)
				}
			} else {
				if err := diff(ctx, buf, origPath, pkg.Fset, f); err != nil {
					errorList = append(errorList, err)
				}
			}
		}
	}

	for _, err := range errorList {
		fmt.Fprintf(os.Stderr, "%s: %v\n", programName, err)
	}
	if err := installModule(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", programName, err)
	}
	if len(errorList) > 0 {
		return 1
	}
	return 0
}

type symbol struct {
	importPath string
	typeName   string
	name       string
}

var importRemaps = map[string]string{
	"crawshaw.io/sqlite":                        "zombiezen.com/go/sqlite",
	"crawshaw.io/sqlite/sqlitex":                "zombiezen.com/go/sqlite/sqlitex",
	"zombiezen.com/go/bass/sql/sqlitefile":      "zombiezen.com/go/sqlite/sqlitefile",
	"zombiezen.com/go/bass/sql/sqlitemigration": "zombiezen.com/go/sqlite/sqlitemigration",
}

var symbolRewrites = map[symbol]string{
	{"crawshaw.io/sqlite", "", "ErrorCode"}:         "ResultCode",
	{"crawshaw.io/sqlite", "Conn", "GetAutocommit"}: "AutocommitEnabled",

	// OpenFlags
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_READONLY"}:       "OpenReadOnly",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_READWRITE"}:      "OpenReadWrite",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_CREATE"}:         "OpenCreate",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_URI"}:            "OpenURI",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_MEMORY"}:         "OpenMemory",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_MAIN_DB"}:        "OpenMainDB",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_TEMP_DB"}:        "OpenTempDB",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_TRANSIENT_DB"}:   "OpenTransientDB",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_MAIN_JOURNAL"}:   "OpenMainJournal",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_TEMP_JOURNAL"}:   "OpenTempJournal",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_SUBJOURNAL"}:     "OpenSubjournal",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_MASTER_JOURNAL"}: "OpenMasterJournal",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_NOMUTEX"}:        "OpenNoMutex",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_FULLMUTEX"}:      "OpenFullMutex",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_SHAREDCACHE"}:    "OpenSharedCache",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_PRIVATECACHE"}:   "OpenPrivateCache",
	{"crawshaw.io/sqlite", "", "SQLITE_OPEN_WAL"}:            "OpenWAL",

	// ColumnType
	{"crawshaw.io/sqlite", "", "SQLITE_INTEGER"}: "TypeInteger",
	{"crawshaw.io/sqlite", "", "SQLITE_FLOAT"}:   "TypeFloat",
	{"crawshaw.io/sqlite", "", "SQLITE_TEXT"}:    "TypeText",
	{"crawshaw.io/sqlite", "", "SQLITE_BLOB"}:    "TypeBlob",
	{"crawshaw.io/sqlite", "", "SQLITE_NULL"}:    "TypeNull",

	// Primary result codes.
	{"crawshaw.io/sqlite", "", "SQLITE_OK"}:         "ResultOK",
	{"crawshaw.io/sqlite", "", "SQLITE_ERROR"}:      "ResultError",
	{"crawshaw.io/sqlite", "", "SQLITE_INTERNAL"}:   "ResultInternal",
	{"crawshaw.io/sqlite", "", "SQLITE_PERM"}:       "ResultPerm",
	{"crawshaw.io/sqlite", "", "SQLITE_ABORT"}:      "ResultAbort",
	{"crawshaw.io/sqlite", "", "SQLITE_BUSY"}:       "ResultBusy",
	{"crawshaw.io/sqlite", "", "SQLITE_LOCKED"}:     "ResultLocked",
	{"crawshaw.io/sqlite", "", "SQLITE_NOMEM"}:      "ResultNoMem",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY"}:   "ResultReadOnly",
	{"crawshaw.io/sqlite", "", "SQLITE_INTERRUPT"}:  "ResultInterrupt",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR"}:      "ResultIOErr",
	{"crawshaw.io/sqlite", "", "SQLITE_CORRUPT"}:    "ResultCorrupt",
	{"crawshaw.io/sqlite", "", "SQLITE_NOTFOUND"}:   "ResultNotFound",
	{"crawshaw.io/sqlite", "", "SQLITE_FULL"}:       "ResultFull",
	{"crawshaw.io/sqlite", "", "SQLITE_CANTOPEN"}:   "ResultCantOpen",
	{"crawshaw.io/sqlite", "", "SQLITE_PROTOCOL"}:   "ResultProtocol",
	{"crawshaw.io/sqlite", "", "SQLITE_EMPTY"}:      "ResultEmpty",
	{"crawshaw.io/sqlite", "", "SQLITE_SCHEMA"}:     "ResultSchema",
	{"crawshaw.io/sqlite", "", "SQLITE_TOOBIG"}:     "ResultTooBig",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT"}: "ResultConstraint",
	{"crawshaw.io/sqlite", "", "SQLITE_MISMATCH"}:   "ResultMismatch",
	{"crawshaw.io/sqlite", "", "SQLITE_MISUSE"}:     "ResultMisuse",
	{"crawshaw.io/sqlite", "", "SQLITE_NOLFS"}:      "ResultNoLFS",
	{"crawshaw.io/sqlite", "", "SQLITE_AUTH"}:       "ResultAuth",
	{"crawshaw.io/sqlite", "", "SQLITE_FORMAT"}:     "ResultFormat",
	{"crawshaw.io/sqlite", "", "SQLITE_RANGE"}:      "ResultRange",
	{"crawshaw.io/sqlite", "", "SQLITE_NOTADB"}:     "ResultNotADB",
	{"crawshaw.io/sqlite", "", "SQLITE_NOTICE"}:     "ResultNotice",
	{"crawshaw.io/sqlite", "", "SQLITE_WARNING"}:    "ResultWarning",
	{"crawshaw.io/sqlite", "", "SQLITE_ROW"}:        "ResultRow",
	{"crawshaw.io/sqlite", "", "SQLITE_DONE"}:       "ResultDone",

	// Extended result codes.
	{"crawshaw.io/sqlite", "", "SQLITE_ERROR_MISSING_COLLSEQ"}:   "ResultErrorMissingCollSeq",
	{"crawshaw.io/sqlite", "", "SQLITE_ERROR_RETRY"}:             "ResultErrorRetry",
	{"crawshaw.io/sqlite", "", "SQLITE_ERROR_SNAPSHOT"}:          "ResultErrorSnapshot",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_READ"}:              "ResultIOErrRead",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SHORT_READ"}:        "ResultIOErrShortRead",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_WRITE"}:             "ResultIOErrWrite",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_FSYNC"}:             "ResultIOErrFsync",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_DIR_FSYNC"}:         "ResultIOErrDirFsync",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_TRUNCATE"}:          "ResultIOErrTruncate",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_FSTAT"}:             "ResultIOErrFstat",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_UNLOCK"}:            "ResultIOErrUnlock",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_RDLOCK"}:            "ResultIOErrReadLock",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_DELETE"}:            "ResultIOErrDelete",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_BLOCKED"}:           "ResultIOErrBlocked",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_NOMEM"}:             "ResultIOErrNoMem",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_ACCESS"}:            "ResultIOErrAccess",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_CHECKRESERVEDLOCK"}: "ResultIOErrCheckReservedLock",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_LOCK"}:              "ResultIOErrLock",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_CLOSE"}:             "ResultIOErrClose",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_DIR_CLOSE"}:         "ResultIOErrDirClose",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SHMOPEN"}:           "ResultIOErrSHMOpen",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SHMSIZE"}:           "ResultIOErrSHMSize",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SHMLOCK"}:           "ResultIOErrSHMLock",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SHMMAP"}:            "ResultIOErrSHMMap",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_SEEK"}:              "ResultIOErrSeek",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_DELETE_NOENT"}:      "ResultIOErrDeleteNoEnt",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_MMAP"}:              "ResultIOErrMMap",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_GETTEMPPATH"}:       "ResultIOErrGetTempPath",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_CONVPATH"}:          "ResultIOErrConvPath",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_VNODE"}:             "ResultIOErrVNode",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_AUTH"}:              "ResultIOErrAuth",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_BEGIN_ATOMIC"}:      "ResultIOErrBeginAtomic",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_COMMIT_ATOMIC"}:     "ResultIOErrCommitAtomic",
	{"crawshaw.io/sqlite", "", "SQLITE_IOERR_ROLLBACK_ATOMIC"}:   "ResultIOErrRollbackAtomic",
	{"crawshaw.io/sqlite", "", "SQLITE_LOCKED_SHAREDCACHE"}:      "ResultLockedSharedCache",
	{"crawshaw.io/sqlite", "", "SQLITE_BUSY_RECOVERY"}:           "ResultBusyRecovery",
	{"crawshaw.io/sqlite", "", "SQLITE_BUSY_SNAPSHOT"}:           "ResultBusySnapshot",
	{"crawshaw.io/sqlite", "", "SQLITE_CANTOPEN_NOTEMPDIR"}:      "ResultCantOpenNoTempDir",
	{"crawshaw.io/sqlite", "", "SQLITE_CANTOPEN_ISDIR"}:          "ResultCantOpenIsDir",
	{"crawshaw.io/sqlite", "", "SQLITE_CANTOPEN_FULLPATH"}:       "ResultCantOpenFullPath",
	{"crawshaw.io/sqlite", "", "SQLITE_CANTOPEN_CONVPATH"}:       "ResultCantOpenConvPath",
	{"crawshaw.io/sqlite", "", "SQLITE_CORRUPT_VTAB"}:            "ResultCorruptVTab",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_RECOVERY"}:       "ResultReadOnlyRecovery",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_CANTLOCK"}:       "ResultReadOnlyCantLock",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_ROLLBACK"}:       "ResultReadOnlyRollback",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_DBMOVED"}:        "ResultReadOnlyDBMoved",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_CANTINIT"}:       "ResultReadOnlyCantInit",
	{"crawshaw.io/sqlite", "", "SQLITE_READONLY_DIRECTORY"}:      "ResultReadOnlyDirectory",
	{"crawshaw.io/sqlite", "", "SQLITE_ABORT_ROLLBACK"}:          "ResultAbortRollback",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_CHECK"}:        "ResultConstraintCheck",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_COMMITHOOK"}:   "ResultConstraintCommitHook",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_FOREIGNKEY"}:   "ResultConstraintForeignKey",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_FUNCTION"}:     "ResultConstraintFunction",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_NOTNULL"}:      "ResultConstraintNotNull",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_PRIMARYKEY"}:   "ResultConstraintPrimaryKey",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_TRIGGER"}:      "ResultConstraintTrigger",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_UNIQUE"}:       "ResultConstraintUnique",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_VTAB"}:         "ResultConstraintVTab",
	{"crawshaw.io/sqlite", "", "SQLITE_CONSTRAINT_ROWID"}:        "ResultConstraintRowID",
	{"crawshaw.io/sqlite", "", "SQLITE_NOTICE_RECOVER_WAL"}:      "ResultNoticeRecoverWAL",
	{"crawshaw.io/sqlite", "", "SQLITE_NOTICE_RECOVER_ROLLBACK"}: "ResultNoticeRecoverRollback",
	{"crawshaw.io/sqlite", "", "SQLITE_WARNING_AUTOINDEX"}:       "ResultWarningAutoIndex",
	{"crawshaw.io/sqlite", "", "SQLITE_AUTH_USER"}:               "ResultAuthUser",
}

const (
	removedWarning = "gone with no replacement available"
)

var symbolWarnings = map[symbol]string{
	{"crawshaw.io/sqlite", "Blob", "ReadAt"}:          removedWarning,
	{"crawshaw.io/sqlite", "Blob", "WriteAt"}:         removedWarning,
	{"crawshaw.io/sqlite", "Blob", "Closer"}:          removedWarning,
	{"crawshaw.io/sqlite", "Blob", "ReadWriteSeeker"}: removedWarning,
	{"crawshaw.io/sqlite", "Blob", "ReaderAt"}:        removedWarning,
	{"crawshaw.io/sqlite", "Blob", "WriterAt"}:        removedWarning,

	{"crawshaw.io/sqlite", "", "Error"}: "use sqlite.ErrorCode instead",

	{"crawshaw.io/sqlite", "Conn", "CreateFunction"}:                   "CreateFunction's API has changed substantially and this code needs to be rewritten",
	{"crawshaw.io/sqlite", "Conn", "EnableDoubleQuotedStringLiterals"}: removedWarning,
	{"crawshaw.io/sqlite", "Conn", "EnableLoadExtension"}:              removedWarning,

	{"crawshaw.io/sqlite", "Value", "IsNil"}: removedWarning,
	{"crawshaw.io/sqlite", "Value", "Len"}:   "use Value.Blob or Value.Text methods",
}

const processMode = packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo

func process(pkg *packages.Package, file *ast.File) []error {
	var warnings []error
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		impPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if remapped := importRemaps[impPath]; remapped != "" {
			imp.Path.Value = strconv.Quote(remapped)
		}
	}

	skipList := make(map[ast.Node]struct{})
	for _, decl := range file.Decls {
		astutil.Apply(decl, func(c *astutil.Cursor) bool {
			if _, skip := skipList[c.Node()]; skip {
				return false
			}
			switch node := c.Node().(type) {
			case *ast.Ident:
				obj := pkg.TypesInfo.ObjectOf(node)
				if obj == nil {
					return true
				}
				objPkg := obj.Pkg()
				if objPkg == nil {
					// Universe scope (built-ins).
					return true
				}

				sym := symbol{
					importPath: objPkg.Path(),
					name:       obj.Name(),
				}
				if sig, ok := obj.Type().(*types.Signature); ok {
					if recv := sig.Recv(); recv != nil {
						sym.typeName = depointerType(recv.Type()).(*types.Named).Obj().Name()
					}
				}
				if newName := symbolRewrites[sym]; newName != "" {
					node.Name = newName
				}
				if warning := symbolWarnings[sym]; warning != "" {
					pos := pkg.Fset.Position(node.NamePos)
					warnings = append(warnings, fmt.Errorf("%v: %s", pos, warning))
				}

			case *ast.SelectorExpr:
				sel := pkg.TypesInfo.Selections[node]
				if sel == nil {
					return true
				}
				obj := sel.Obj()
				objPkg := obj.Pkg()
				if objPkg == nil {
					// Universe scope (built-ins).
					return true
				}

				sym := symbol{
					importPath: objPkg.Path(),
					name:       obj.Name(),
				}
				if recv := sel.Recv(); recv != nil {
					if named, ok := depointerType(recv).(*types.Named); ok {
						sym.typeName = named.Obj().Name()
					}
				}
				if newName := symbolRewrites[sym]; newName != "" {
					node.Sel.Name = newName
				}
				if warning := symbolWarnings[sym]; warning != "" {
					pos := pkg.Fset.Position(node.Sel.NamePos)
					warnings = append(warnings, fmt.Errorf("%v: %s", pos, warning))
				}
				// We don't want to visit the identifier during descent,
				// since we've already rewritten it.
				skipList[node.Sel] = struct{}{}
			}
			return true
		}, nil)
	}
	return warnings
}

func depointerType(t types.Type) types.Type {
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

func installModule(ctx context.Context) error {
	listCmd := exec.Command("go", "list", "-m", "zombiezen.com/go/sqlite")
	if err := sigterm.Run(ctx, listCmd); err == nil {
		// Module is already present.
		return nil
	}

	getCmd := exec.Command("go", "get", "-d", "zombiezen.com/go/sqlite@v0.1.0")
	getCmd.Stdout = os.Stderr
	getCmd.Stderr = os.Stderr
	if err := sigterm.Run(ctx, getCmd); err != nil {
		return fmt.Errorf("go get zombiezen.com/go/sqlite: %w", err)
	}
	return nil
}

func write(buf *bytes.Buffer, origPath string, fset *token.FileSet, file *ast.File) error {
	buf.Reset()
	if err := format.Node(buf, fset, file); err != nil {
		return fmt.Errorf("write %s: %w", origPath, err)
	}
	if err := os.WriteFile(origPath, buf.Bytes(), 0o666); err != nil {
		return err
	}
	return nil
}

func diff(ctx context.Context, buf *bytes.Buffer, origPath string, fset *token.FileSet, file *ast.File) error {
	f, err := os.CreateTemp("", "zombiezen-sqlite-*.go")
	if err != nil {
		return fmt.Errorf("diff %s: %w", origPath, err)
	}
	fname := f.Name()
	defer func() {
		f.Close()
		if err := os.Remove(fname); err != nil {
			fmt.Fprintf(os.Stderr, "%s: cleaning up temp file: %v\n", programName, err)
		}
	}()
	if err := format.Node(f, fset, file); err != nil {
		return fmt.Errorf("diff %s %s: %w", origPath, fname, err)
	}
	f.Close()
	buf.Reset()
	c := exec.Command("diff", "-u", origPath, fname)
	c.Stdout = buf
	c.Stderr = os.Stderr
	err = sigterm.Run(ctx, c)
	if err == nil {
		// Files are identical.
		return nil
	}
	if exitErr := new(exec.ExitError); !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		return fmt.Errorf("diff %s %s: %w", origPath, fname, err)
	}
	fmt.Printf("diff -u %s %s\n", origPath, fname)
	os.Stdout.Write(buf.Bytes())
	return nil
}
