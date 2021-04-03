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
	slashpath "path"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	goimports "golang.org/x/tools/imports"
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
				if err := writeFile(buf, origPath, pkg.Fset, f); err != nil {
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
	installFailed := false
	if writeFiles {
		if err := installModule(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", programName, err)
			installFailed = true
		}
	}
	if len(errorList) > 0 || installFailed {
		return 1
	}
	return 0
}

const (
	crawshaw  = "crawshaw.io/sqlite"
	crawshawX = crawshaw + "/sqlitex"

	zombiezen     = "zombiezen.com/go/sqlite"
	zombiezenX    = zombiezen + "/sqlitex"
	zombiezenFile = zombiezen + "/sqlitefile"

	bass     = "zombiezen.com/go/bass/sql"
	bassFile = "zombiezen.com/go/bass/sql/sqlitefile"
)

var importRemaps = map[string]string{
	crawshaw:                  zombiezen,
	crawshawX:                 zombiezenX,
	bass + "/sqlitemigration": zombiezen + "/sqlitemigration",
}

type symbol struct {
	importPath string
	typeName   string
	name       string
}

var symbolRewrites = map[symbol]symbol{
	{crawshaw, "", "ErrorCode"}:         {zombiezen, "", "ResultCode"},
	{crawshaw, "Conn", "GetAutocommit"}: {zombiezen, "", "AutocommitEnabled"},

	// New sqlitefile
	{crawshawX, "", "Buffer"}:        {zombiezenFile, "", "Buffer"},
	{crawshawX, "", "File"}:          {zombiezenFile, "", "File"},
	{crawshawX, "", "NewBuffer"}:     {zombiezenFile, "", "NewBuffer"},
	{crawshawX, "", "NewBufferSize"}: {zombiezenFile, "", "NewBufferSize"},
	{crawshawX, "", "NewFile"}:       {zombiezenFile, "", "NewFile"},
	{crawshawX, "", "NewFileSize"}:   {zombiezenFile, "", "NewFileSize"},

	// bass sqlitefile
	{bassFile, "", "ExecOptions"}:      {zombiezenX, "", "ExecOptions"},
	{bassFile, "", "Exec"}:             {zombiezenX, "", "ExecFS"},
	{bassFile, "", "ExecTransient"}:    {zombiezenX, "", "ExecTransientFS"},
	{bassFile, "", "PrepareTransient"}: {zombiezenX, "", "PrepareTransientFS"},
	{bassFile, "", "ExecScript"}:       {zombiezenX, "", "ExecScriptFS"},

	// OpenFlags
	{crawshaw, "", "SQLITE_OPEN_READONLY"}:       {zombiezen, "", "OpenReadOnly"},
	{crawshaw, "", "SQLITE_OPEN_READWRITE"}:      {zombiezen, "", "OpenReadWrite"},
	{crawshaw, "", "SQLITE_OPEN_CREATE"}:         {zombiezen, "", "OpenCreate"},
	{crawshaw, "", "SQLITE_OPEN_URI"}:            {zombiezen, "", "OpenURI"},
	{crawshaw, "", "SQLITE_OPEN_MEMORY"}:         {zombiezen, "", "OpenMemory"},
	{crawshaw, "", "SQLITE_OPEN_MAIN_DB"}:        {zombiezen, "", "OpenMainDB"},
	{crawshaw, "", "SQLITE_OPEN_TEMP_DB"}:        {zombiezen, "", "OpenTempDB"},
	{crawshaw, "", "SQLITE_OPEN_TRANSIENT_DB"}:   {zombiezen, "", "OpenTransientDB"},
	{crawshaw, "", "SQLITE_OPEN_MAIN_JOURNAL"}:   {zombiezen, "", "OpenMainJournal"},
	{crawshaw, "", "SQLITE_OPEN_TEMP_JOURNAL"}:   {zombiezen, "", "OpenTempJournal"},
	{crawshaw, "", "SQLITE_OPEN_SUBJOURNAL"}:     {zombiezen, "", "OpenSubjournal"},
	{crawshaw, "", "SQLITE_OPEN_MASTER_JOURNAL"}: {zombiezen, "", "OpenMasterJournal"},
	{crawshaw, "", "SQLITE_OPEN_NOMUTEX"}:        {zombiezen, "", "OpenNoMutex"},
	{crawshaw, "", "SQLITE_OPEN_FULLMUTEX"}:      {zombiezen, "", "OpenFullMutex"},
	{crawshaw, "", "SQLITE_OPEN_SHAREDCACHE"}:    {zombiezen, "", "OpenSharedCache"},
	{crawshaw, "", "SQLITE_OPEN_PRIVATECACHE"}:   {zombiezen, "", "OpenPrivateCache"},
	{crawshaw, "", "SQLITE_OPEN_WAL"}:            {zombiezen, "", "OpenWAL"},

	// ColumnType
	{crawshaw, "", "SQLITE_INTEGER"}: {zombiezen, "", "TypeInteger"},
	{crawshaw, "", "SQLITE_FLOAT"}:   {zombiezen, "", "TypeFloat"},
	{crawshaw, "", "SQLITE_TEXT"}:    {zombiezen, "", "TypeText"},
	{crawshaw, "", "SQLITE_BLOB"}:    {zombiezen, "", "TypeBlob"},
	{crawshaw, "", "SQLITE_NULL"}:    {zombiezen, "", "TypeNull"},

	// Primary result codes.
	{crawshaw, "", "SQLITE_OK"}:         {zombiezen, "", "ResultOK"},
	{crawshaw, "", "SQLITE_ERROR"}:      {zombiezen, "", "ResultError"},
	{crawshaw, "", "SQLITE_INTERNAL"}:   {zombiezen, "", "ResultInternal"},
	{crawshaw, "", "SQLITE_PERM"}:       {zombiezen, "", "ResultPerm"},
	{crawshaw, "", "SQLITE_ABORT"}:      {zombiezen, "", "ResultAbort"},
	{crawshaw, "", "SQLITE_BUSY"}:       {zombiezen, "", "ResultBusy"},
	{crawshaw, "", "SQLITE_LOCKED"}:     {zombiezen, "", "ResultLocked"},
	{crawshaw, "", "SQLITE_NOMEM"}:      {zombiezen, "", "ResultNoMem"},
	{crawshaw, "", "SQLITE_READONLY"}:   {zombiezen, "", "ResultReadOnly"},
	{crawshaw, "", "SQLITE_INTERRUPT"}:  {zombiezen, "", "ResultInterrupt"},
	{crawshaw, "", "SQLITE_IOERR"}:      {zombiezen, "", "ResultIOErr"},
	{crawshaw, "", "SQLITE_CORRUPT"}:    {zombiezen, "", "ResultCorrupt"},
	{crawshaw, "", "SQLITE_NOTFOUND"}:   {zombiezen, "", "ResultNotFound"},
	{crawshaw, "", "SQLITE_FULL"}:       {zombiezen, "", "ResultFull"},
	{crawshaw, "", "SQLITE_CANTOPEN"}:   {zombiezen, "", "ResultCantOpen"},
	{crawshaw, "", "SQLITE_PROTOCOL"}:   {zombiezen, "", "ResultProtocol"},
	{crawshaw, "", "SQLITE_EMPTY"}:      {zombiezen, "", "ResultEmpty"},
	{crawshaw, "", "SQLITE_SCHEMA"}:     {zombiezen, "", "ResultSchema"},
	{crawshaw, "", "SQLITE_TOOBIG"}:     {zombiezen, "", "ResultTooBig"},
	{crawshaw, "", "SQLITE_CONSTRAINT"}: {zombiezen, "", "ResultConstraint"},
	{crawshaw, "", "SQLITE_MISMATCH"}:   {zombiezen, "", "ResultMismatch"},
	{crawshaw, "", "SQLITE_MISUSE"}:     {zombiezen, "", "ResultMisuse"},
	{crawshaw, "", "SQLITE_NOLFS"}:      {zombiezen, "", "ResultNoLFS"},
	{crawshaw, "", "SQLITE_AUTH"}:       {zombiezen, "", "ResultAuth"},
	{crawshaw, "", "SQLITE_FORMAT"}:     {zombiezen, "", "ResultFormat"},
	{crawshaw, "", "SQLITE_RANGE"}:      {zombiezen, "", "ResultRange"},
	{crawshaw, "", "SQLITE_NOTADB"}:     {zombiezen, "", "ResultNotADB"},
	{crawshaw, "", "SQLITE_NOTICE"}:     {zombiezen, "", "ResultNotice"},
	{crawshaw, "", "SQLITE_WARNING"}:    {zombiezen, "", "ResultWarning"},
	{crawshaw, "", "SQLITE_ROW"}:        {zombiezen, "", "ResultRow"},
	{crawshaw, "", "SQLITE_DONE"}:       {zombiezen, "", "ResultDone"},

	// Extended result codes.
	{crawshaw, "", "SQLITE_ERROR_MISSING_COLLSEQ"}:   {zombiezen, "", "ResultErrorMissingCollSeq"},
	{crawshaw, "", "SQLITE_ERROR_RETRY"}:             {zombiezen, "", "ResultErrorRetry"},
	{crawshaw, "", "SQLITE_ERROR_SNAPSHOT"}:          {zombiezen, "", "ResultErrorSnapshot"},
	{crawshaw, "", "SQLITE_IOERR_READ"}:              {zombiezen, "", "ResultIOErrRead"},
	{crawshaw, "", "SQLITE_IOERR_SHORT_READ"}:        {zombiezen, "", "ResultIOErrShortRead"},
	{crawshaw, "", "SQLITE_IOERR_WRITE"}:             {zombiezen, "", "ResultIOErrWrite"},
	{crawshaw, "", "SQLITE_IOERR_FSYNC"}:             {zombiezen, "", "ResultIOErrFsync"},
	{crawshaw, "", "SQLITE_IOERR_DIR_FSYNC"}:         {zombiezen, "", "ResultIOErrDirFsync"},
	{crawshaw, "", "SQLITE_IOERR_TRUNCATE"}:          {zombiezen, "", "ResultIOErrTruncate"},
	{crawshaw, "", "SQLITE_IOERR_FSTAT"}:             {zombiezen, "", "ResultIOErrFstat"},
	{crawshaw, "", "SQLITE_IOERR_UNLOCK"}:            {zombiezen, "", "ResultIOErrUnlock"},
	{crawshaw, "", "SQLITE_IOERR_RDLOCK"}:            {zombiezen, "", "ResultIOErrReadLock"},
	{crawshaw, "", "SQLITE_IOERR_DELETE"}:            {zombiezen, "", "ResultIOErrDelete"},
	{crawshaw, "", "SQLITE_IOERR_BLOCKED"}:           {zombiezen, "", "ResultIOErrBlocked"},
	{crawshaw, "", "SQLITE_IOERR_NOMEM"}:             {zombiezen, "", "ResultIOErrNoMem"},
	{crawshaw, "", "SQLITE_IOERR_ACCESS"}:            {zombiezen, "", "ResultIOErrAccess"},
	{crawshaw, "", "SQLITE_IOERR_CHECKRESERVEDLOCK"}: {zombiezen, "", "ResultIOErrCheckReservedLock"},
	{crawshaw, "", "SQLITE_IOERR_LOCK"}:              {zombiezen, "", "ResultIOErrLock"},
	{crawshaw, "", "SQLITE_IOERR_CLOSE"}:             {zombiezen, "", "ResultIOErrClose"},
	{crawshaw, "", "SQLITE_IOERR_DIR_CLOSE"}:         {zombiezen, "", "ResultIOErrDirClose"},
	{crawshaw, "", "SQLITE_IOERR_SHMOPEN"}:           {zombiezen, "", "ResultIOErrSHMOpen"},
	{crawshaw, "", "SQLITE_IOERR_SHMSIZE"}:           {zombiezen, "", "ResultIOErrSHMSize"},
	{crawshaw, "", "SQLITE_IOERR_SHMLOCK"}:           {zombiezen, "", "ResultIOErrSHMLock"},
	{crawshaw, "", "SQLITE_IOERR_SHMMAP"}:            {zombiezen, "", "ResultIOErrSHMMap"},
	{crawshaw, "", "SQLITE_IOERR_SEEK"}:              {zombiezen, "", "ResultIOErrSeek"},
	{crawshaw, "", "SQLITE_IOERR_DELETE_NOENT"}:      {zombiezen, "", "ResultIOErrDeleteNoEnt"},
	{crawshaw, "", "SQLITE_IOERR_MMAP"}:              {zombiezen, "", "ResultIOErrMMap"},
	{crawshaw, "", "SQLITE_IOERR_GETTEMPPATH"}:       {zombiezen, "", "ResultIOErrGetTempPath"},
	{crawshaw, "", "SQLITE_IOERR_CONVPATH"}:          {zombiezen, "", "ResultIOErrConvPath"},
	{crawshaw, "", "SQLITE_IOERR_VNODE"}:             {zombiezen, "", "ResultIOErrVNode"},
	{crawshaw, "", "SQLITE_IOERR_AUTH"}:              {zombiezen, "", "ResultIOErrAuth"},
	{crawshaw, "", "SQLITE_IOERR_BEGIN_ATOMIC"}:      {zombiezen, "", "ResultIOErrBeginAtomic"},
	{crawshaw, "", "SQLITE_IOERR_COMMIT_ATOMIC"}:     {zombiezen, "", "ResultIOErrCommitAtomic"},
	{crawshaw, "", "SQLITE_IOERR_ROLLBACK_ATOMIC"}:   {zombiezen, "", "ResultIOErrRollbackAtomic"},
	{crawshaw, "", "SQLITE_LOCKED_SHAREDCACHE"}:      {zombiezen, "", "ResultLockedSharedCache"},
	{crawshaw, "", "SQLITE_BUSY_RECOVERY"}:           {zombiezen, "", "ResultBusyRecovery"},
	{crawshaw, "", "SQLITE_BUSY_SNAPSHOT"}:           {zombiezen, "", "ResultBusySnapshot"},
	{crawshaw, "", "SQLITE_CANTOPEN_NOTEMPDIR"}:      {zombiezen, "", "ResultCantOpenNoTempDir"},
	{crawshaw, "", "SQLITE_CANTOPEN_ISDIR"}:          {zombiezen, "", "ResultCantOpenIsDir"},
	{crawshaw, "", "SQLITE_CANTOPEN_FULLPATH"}:       {zombiezen, "", "ResultCantOpenFullPath"},
	{crawshaw, "", "SQLITE_CANTOPEN_CONVPATH"}:       {zombiezen, "", "ResultCantOpenConvPath"},
	{crawshaw, "", "SQLITE_CORRUPT_VTAB"}:            {zombiezen, "", "ResultCorruptVTab"},
	{crawshaw, "", "SQLITE_READONLY_RECOVERY"}:       {zombiezen, "", "ResultReadOnlyRecovery"},
	{crawshaw, "", "SQLITE_READONLY_CANTLOCK"}:       {zombiezen, "", "ResultReadOnlyCantLock"},
	{crawshaw, "", "SQLITE_READONLY_ROLLBACK"}:       {zombiezen, "", "ResultReadOnlyRollback"},
	{crawshaw, "", "SQLITE_READONLY_DBMOVED"}:        {zombiezen, "", "ResultReadOnlyDBMoved"},
	{crawshaw, "", "SQLITE_READONLY_CANTINIT"}:       {zombiezen, "", "ResultReadOnlyCantInit"},
	{crawshaw, "", "SQLITE_READONLY_DIRECTORY"}:      {zombiezen, "", "ResultReadOnlyDirectory"},
	{crawshaw, "", "SQLITE_ABORT_ROLLBACK"}:          {zombiezen, "", "ResultAbortRollback"},
	{crawshaw, "", "SQLITE_CONSTRAINT_CHECK"}:        {zombiezen, "", "ResultConstraintCheck"},
	{crawshaw, "", "SQLITE_CONSTRAINT_COMMITHOOK"}:   {zombiezen, "", "ResultConstraintCommitHook"},
	{crawshaw, "", "SQLITE_CONSTRAINT_FOREIGNKEY"}:   {zombiezen, "", "ResultConstraintForeignKey"},
	{crawshaw, "", "SQLITE_CONSTRAINT_FUNCTION"}:     {zombiezen, "", "ResultConstraintFunction"},
	{crawshaw, "", "SQLITE_CONSTRAINT_NOTNULL"}:      {zombiezen, "", "ResultConstraintNotNull"},
	{crawshaw, "", "SQLITE_CONSTRAINT_PRIMARYKEY"}:   {zombiezen, "", "ResultConstraintPrimaryKey"},
	{crawshaw, "", "SQLITE_CONSTRAINT_TRIGGER"}:      {zombiezen, "", "ResultConstraintTrigger"},
	{crawshaw, "", "SQLITE_CONSTRAINT_UNIQUE"}:       {zombiezen, "", "ResultConstraintUnique"},
	{crawshaw, "", "SQLITE_CONSTRAINT_VTAB"}:         {zombiezen, "", "ResultConstraintVTab"},
	{crawshaw, "", "SQLITE_CONSTRAINT_ROWID"}:        {zombiezen, "", "ResultConstraintRowID"},
	{crawshaw, "", "SQLITE_NOTICE_RECOVER_WAL"}:      {zombiezen, "", "ResultNoticeRecoverWAL"},
	{crawshaw, "", "SQLITE_NOTICE_RECOVER_ROLLBACK"}: {zombiezen, "", "ResultNoticeRecoverRollback"},
	{crawshaw, "", "SQLITE_WARNING_AUTOINDEX"}:       {zombiezen, "", "ResultWarningAutoIndex"},
	{crawshaw, "", "SQLITE_AUTH_USER"}:               {zombiezen, "", "ResultAuthUser"},
}

const (
	removedWarning = "gone with no replacement available"
)

var symbolWarnings = map[symbol]string{
	{crawshaw, "Blob", "ReadAt"}:          removedWarning,
	{crawshaw, "Blob", "WriteAt"}:         removedWarning,
	{crawshaw, "Blob", "Closer"}:          removedWarning,
	{crawshaw, "Blob", "ReadWriteSeeker"}: removedWarning,
	{crawshaw, "Blob", "ReaderAt"}:        removedWarning,
	{crawshaw, "Blob", "WriterAt"}:        removedWarning,

	{crawshaw, "", "Error"}: "use sqlite.ErrorCode instead",

	{crawshaw, "Conn", "CreateFunction"}:                   "CreateFunction's API has changed substantially and this code needs to be rewritten",
	{crawshaw, "Conn", "EnableDoubleQuotedStringLiterals"}: removedWarning,
	{crawshaw, "Conn", "EnableLoadExtension"}:              removedWarning,

	{crawshaw, "Value", "IsNil"}: removedWarning,
	{crawshaw, "Value", "Len"}:   "use Value.Blob or Value.Text methods",
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
			if local := localImportName(pkg.TypesInfo, imp); local != slashpath.Base(remapped) {
				if imp.Name == nil {
					imp.Name = ast.NewIdent(local)
				} else {
					imp.Name.Name = local
				}
			}
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
				if newSym := symbolRewrites[sym]; newSym.name != "" {
					node.Name = newSym.name
					if newSym.importPath != importRemaps[sym.importPath] {
						// Your symbol is in a different package, Mario!
						if sel, ok := c.Parent().(*ast.SelectorExpr); ok {
							if pkgIdent, ok := sel.X.(*ast.Ident); ok {
								// Qualified identifier.
								pkgIdent.Name = acquirePackageID(pkg.Fset, pkg.TypesInfo, file, newSym.importPath)
							}
						}
					}
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
				if newSym := symbolRewrites[sym]; newSym.name != "" {
					// Selections doesn't include qualified identifiers,
					// so no import changes.
					node.Sel.Name = newSym.name
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

func acquirePackageID(fset *token.FileSet, info *types.Info, file *ast.File, importPath string) string {
	usedIDs := make(map[string]struct{}, len(file.Imports))
	for _, imp := range file.Imports {
		ipath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		id := localImportName(info, imp)
		if id == "_" {
			continue
		}
		if ipath == importPath {
			return id
		}
		usedIDs[id] = struct{}{}
	}

	base := slashpath.Base(importPath)
	if _, baseUsed := usedIDs[base]; !baseUsed {
		astutil.AddImport(fset, file, importPath)
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s%d", base, i)
		if _, used := usedIDs[id]; !used {
			astutil.AddNamedImport(fset, file, id, importPath)
			return id
		}
	}
}

// localImportName returns the identifier being used for an import declaration.
func localImportName(info *types.Info, imp *ast.ImportSpec) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	if resolvedName, ok := info.Implicits[imp].(*types.PkgName); ok {
		return resolvedName.Name()
	}
	// Fallback: Use last path component of import path if it is an identifier.
	path, err := strconv.Unquote(imp.Path.Value)
	if err != nil {
		return ""
	}
	base := slashpath.Base(path)
	if !token.IsIdentifier(base) {
		return ""
	}
	return base
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
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx,
		Mode:    packages.NeedName | packages.NeedFiles,
	}, zombiezen)
	if err != nil {
		return fmt.Errorf("go get %s: %w", zombiezen, err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) == 0 && pkg.PkgPath == zombiezen {
			// Already installed.
			return nil
		}
	}

	getCmd := exec.Command("go", "get", "-d", zombiezen+"@v0.1.0")
	getCmd.Stdout = os.Stderr
	getCmd.Stderr = os.Stderr
	if err := sigterm.Run(ctx, getCmd); err != nil {
		return fmt.Errorf("go get %s: %w", zombiezen, err)
	}
	return nil
}

func formatFile(buf *bytes.Buffer, path string, fset *token.FileSet, file *ast.File) ([]byte, error) {
	buf.Reset()
	if err := format.Node(buf, fset, file); err != nil {
		return nil, err
	}
	return goimports.Process(path, buf.Bytes(), nil)
}

func writeFile(buf *bytes.Buffer, origPath string, fset *token.FileSet, file *ast.File) error {
	formatted, err := formatFile(buf, origPath, fset, file)
	if err != nil {
		return fmt.Errorf("write %s: %w", origPath, err)
	}
	if err := os.WriteFile(origPath, formatted, 0o666); err != nil {
		return err
	}
	return nil
}

func diff(ctx context.Context, buf *bytes.Buffer, origPath string, fset *token.FileSet, file *ast.File) error {
	formatted, err := formatFile(buf, origPath, fset, file)
	if err != nil {
		return fmt.Errorf("diff %s: %w", origPath, err)
	}
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
	if _, err := f.Write(formatted); err != nil {
		return fmt.Errorf("diff %s %s: %w", origPath, fname, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("diff %s %s: %w", origPath, fname, err)
	}
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
