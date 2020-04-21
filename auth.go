package sqlite

// #include <stdint.h>
// #include <sqlite3.h>
// extern int go_sqlite_auth_tramp(uintptr_t, int, char*, char*, char*, char*);
// static int c_auth_tramp(void *userData, int action, const char* arg1, const char* arg2, const char* db, const char* trigger) {
//   return go_sqlite_auth_tramp((uintptr_t)userData, action, (char*)arg1, (char*)arg2, (char*)db, (char*)trigger);
// }
// static int sqlite3_go_set_authorizer(sqlite3* conn, uintptr_t id) {
//   return sqlite3_set_authorizer(conn, c_auth_tramp, (void*)id);
// }
import "C"
import (
	"errors"
	"sync"
)

// An Authorizer is called during statement preparation to see whether an action
// is allowed by the application. See https://sqlite.org/c3ref/set_authorizer.html
type Authorizer interface {
	Authorize(action OpType, info ActionInfo) AuthResult
}

// ActionInfo holds information about an action to be authorized.
type ActionInfo struct {
	Arg1     string
	Arg2     string
	Database string
	Trigger  string
}

// SetAuthorizer registers an authorizer for the database connection.
// SetAuthorizer(nil) clears any authorizer previously set.
func (conn *Conn) SetAuthorizer(auth Authorizer) error {
	if auth == nil {
		if conn.authorizer == -1 {
			return nil
		}
		conn.releaseAuthorizer()
		res := C.sqlite3_set_authorizer(conn.conn, nil, nil)
		return reserr("SetAuthorizer", "", "", res)
	}

	authFuncs.mu.Lock()
	id := authFuncs.next
	next := authFuncs.next + 1
	if next < 0 {
		authFuncs.mu.Unlock()
		return errors.New("sqlite: authorizer function id overflow")
	}
	authFuncs.next = next
	authFuncs.m[id] = auth
	authFuncs.mu.Unlock()

	res := C.sqlite3_go_set_authorizer(conn.conn, C.uintptr_t(id))
	return reserr("SetAuthorizer", "", "", res)
}

func (conn *Conn) releaseAuthorizer() {
	if conn.authorizer == -1 {
		return
	}
	authFuncs.mu.Lock()
	delete(authFuncs.m, conn.authorizer)
	authFuncs.mu.Unlock()
	conn.authorizer = -1
}

var authFuncs = struct {
	mu   sync.RWMutex
	m    map[int]Authorizer
	next int
}{
	m: make(map[int]Authorizer),
}

//export go_sqlite_auth_tramp
func go_sqlite_auth_tramp(id uintptr, action C.int, arg1, arg2 *C.char, db *C.char, trigger *C.char) C.int {
	authFuncs.mu.RLock()
	auth := authFuncs.m[int(id)]
	authFuncs.mu.RUnlock()
	info := ActionInfo{}
	if arg1 != nil {
		info.Arg1 = C.GoString(arg1)
	}
	if arg2 != nil {
		info.Arg2 = C.GoString(arg2)
	}
	if db != nil {
		info.Database = C.GoString(db)
	}
	if trigger != nil {
		info.Trigger = C.GoString(trigger)
	}
	return C.int(auth.Authorize(OpType(action), info))
}

// AuthorizeFunc is a function that implements Authorizer.
type AuthorizeFunc func(action OpType, info ActionInfo) AuthResult

// Authorize calls f.
func (f AuthorizeFunc) Authorize(action OpType, info ActionInfo) AuthResult {
	return f(action, info)
}

// AuthResult is the result of a call to an Authorizer. The zero value is
// SQLITE_OK.
type AuthResult int

// Possible return values of an Authorizer.
const (
	// Cause the entire SQL statement to be rejected with an error.
	SQLITE_DENY = AuthResult(C.SQLITE_DENY)
	// Disallow the specific action but allow the SQL statement to continue to
	// be compiled.
	SQLITE_IGNORE = AuthResult(C.SQLITE_IGNORE)
)

// String returns the C constant name of the result.
func (result AuthResult) String() string {
	switch result {
	default:
		var buf [20]byte
		return "SQLITE_UNKNOWN_AUTH_RESULT(" + string(itoa(buf[:], int64(result))) + ")"
	case AuthResult(C.SQLITE_OK):
		return "SQLITE_OK"
	case SQLITE_DENY:
		return "SQLITE_DENY"
	case SQLITE_IGNORE:
		return "SQLITE_IGNORE"
	}
}
