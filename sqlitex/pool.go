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
	"context"
	"runtime/trace"
	"sync"

	"crawshaw.io/sqlite"
)

// Pool is a pool of SQLite connections.
//
// It is safe for use by multiple goroutines concurrently.
//
// Typically, a goroutine that needs to use an SQLite *Conn
// Gets it from the pool and defers its return:
//
//	conn := dbpool.Get(nil)
//	defer dbpool.Put(conn)
//
// As Get may block, a context can be used to return if a task
// is cancelled. In this case the Conn returned will be nil:
//
//	conn := dbpool.Get(ctx)
//	if conn == nil {
//		return context.Canceled
//	}
//	defer dbpool.Put(conn)
type Pool struct {
	// If checkReset, the Put method checks all of the connection's
	// prepared statements and ensures they were correctly cleaned up.
	// If they were not, Put will panic with details.
	//
	// TODO: export this? Is it enough of a performance concern?
	checkReset bool

	free   chan *sqlite.Conn
	closed chan struct{}

	allMu sync.Mutex
	all   map[*sqlite.Conn]struct{}
}

// Open opens a fixed-size pool of SQLite connections.
// A flags value of 0 defaults to:
//
//	SQLITE_OPEN_READWRITE
//	SQLITE_OPEN_CREATE
//	SQLITE_OPEN_WAL
//	SQLITE_OPEN_URI
//	SQLITE_OPEN_NOMUTEX
func Open(uri string, flags sqlite.OpenFlags, poolSize int) (*Pool, error) {
	if uri == ":memory:" {
		return nil, strerror{msg: `sqlite: ":memory:" does not work with multiple connections, use "file::memory:?mode=memory"`}
	}

	p := &Pool{
		checkReset: true,
		free:       make(chan *sqlite.Conn, poolSize),
		closed:     make(chan struct{}),
	}

	if flags == 0 {
		flags = sqlite.SQLITE_OPEN_READWRITE | sqlite.SQLITE_OPEN_CREATE | sqlite.SQLITE_OPEN_WAL | sqlite.SQLITE_OPEN_URI | sqlite.SQLITE_OPEN_NOMUTEX
	}

	// sqlitex_pool is also defined in package sqlite
	const sqlitex_pool = sqlite.OpenFlags(0x01000000)
	flags &= sqlitex_pool

	p.allMu.Lock()
	defer p.allMu.Unlock()
	p.all = make(map[*sqlite.Conn]struct{})
	for i := 0; i < poolSize; i++ {
		conn, err := sqlite.OpenConn(uri, flags)
		if err != nil {
			p.Close()
			return nil, err
		}
		p.free <- conn
		p.all[conn] = struct{}{}
	}

	return p, nil
}

// Get gets an SQLite connection from the pool.
//
// If no Conn is available, Get will block until one is,
// or until either the Pool is closed or the context
// expires.
//
// The provided context is used to control the execution
// lifetime of the connection. See Conn.SetInterrupt for
// details.
func (p *Pool) Get(ctx context.Context) *sqlite.Conn {
	var tr sqlite.Tracer
	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
		tr = &tracer{ctx: ctx}
	}
	select {
	case conn, ok := <-p.free:
		if !ok {
			return nil // pool is closed
		}
		conn.SetTracer(tr)
		conn.SetInterrupt(doneCh)
		return conn
	case <-doneCh:
		return nil
	case <-p.closed:
		return nil
	}
}

// Put puts an SQLite connection back into the Pool.
// A nil conn will cause Put to panic.
func (p *Pool) Put(conn *sqlite.Conn) {
	if conn == nil {
		panic("attempted to Put a nil Conn into Pool")
	}
	if p.checkReset {
		query := conn.CheckReset()
		if query != "" {
			panic("connection returned to pool has active statement: \"" + query + "\"")
		}
	}

	p.allMu.Lock()
	_, found := p.all[conn]
	p.allMu.Unlock()

	if !found {
		panic("sqlite.Pool.Put: connection not created by this pool")
	}

	conn.SetTracer(nil)
	conn.SetInterrupt(nil)
	select {
	case p.free <- conn:
	default:
	}
}

// Close closes all the connections in the Pool.
func (p *Pool) Close() (err error) {
	close(p.closed)

	p.allMu.Lock()
	for conn := range p.all {
		err2 := conn.Close()
		if err == nil {
			err = err2
		}
	}
	p.allMu.Unlock()

	close(p.free)
	for range p.free {
	}
	return err
}

type strerror struct {
	msg string
}

func (err strerror) Error() string { return err.msg }

type tracer struct {
	ctx       context.Context
	ctxStack  []context.Context
	taskStack []*trace.Task
}

func (t *tracer) pctx() context.Context {
	if len(t.ctxStack) != 0 {
		return t.ctxStack[len(t.ctxStack)-1]
	}
	return t.ctx
}

func (t *tracer) Push(name string) {
	ctx, task := trace.NewTask(t.pctx(), name)
	t.ctxStack = append(t.ctxStack, ctx)
	t.taskStack = append(t.taskStack, task)
}

func (t *tracer) Pop() {
	t.taskStack[len(t.taskStack)-1].End()
	t.taskStack = t.taskStack[:len(t.taskStack)-1]
	t.ctxStack = t.ctxStack[:len(t.ctxStack)-1]
}

func (t *tracer) NewTask(name string) sqlite.TracerTask {
	ctx, task := trace.NewTask(t.pctx(), name)
	return &tracerTask{
		ctx:  ctx,
		task: task,
	}
}

type tracerTask struct {
	ctx    context.Context
	task   *trace.Task
	region *trace.Region
}

func (t *tracerTask) StartRegion(regionType string) {
	if t.region != nil {
		panic("sqlitex.tracerTask.StartRegion: already in region")
	}
	t.region = trace.StartRegion(t.ctx, regionType)
}

func (t *tracerTask) EndRegion() {
	t.region.End()
	t.region = nil
}

func (t *tracerTask) End() {
	t.task.End()
}
