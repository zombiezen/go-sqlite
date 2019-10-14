package sqlitex

import (
	"context"
	"runtime"

	"crawshaw.io/sqlite"
)

// GetSnapshot returns a Snapshot that should remain available for reads until
// it is garbage collected. This is achieved by setting aside a conn from the
// pool and keeping a read transaction open on it until the Snapshot is garbage
// collected. When the snapshot is garbage collected, the read transaction is
// closed, the conn is returned to the pool, and the Snapshot is freed. Thus,
// until the returned Snapshot is garbage collected, the Pool will have one
// fewer Conn, and it should not be possible for the WAL to be checkpointed
// beyond the point of the Snapshot.
//
// It is safe for p.Close to be called before the Snapshot is garbage
// collected.
func (p *Pool) GetSnapshot(ctx context.Context) (*sqlite.Snapshot, error) {
	conn := p.Get(ctx)
	if conn == nil {
		return nil, context.Canceled
	}
	conn.SetInterrupt(nil)
	s, release, err := conn.GetSnapshot("main")
	if err != nil {
		return nil, err
	}
	runtime.SetFinalizer(s, nil)
	runtime.SetFinalizer(s, func(s *sqlite.Snapshot) {
		// Free the C resources associated with the Snapshot.
		s.Free()
		// Allow the WAL to be checkpointed past the point of
		// the Snapshot.
		release()
		// Return the conn to the Pool for reuse.
		p.Put(conn)
	})
	return s, nil
}
