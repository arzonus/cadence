package warmstorage

import (
	"context"
	"sync"

	"github.com/uber/cadence/common/persistence"
)

// lazyClosedStore opens the closed store on first use, so a cluster that never enables warm storage doesn't pay
// for the connection (on Cassandra, a second session per process).
type lazyClosedStore struct {
	open func() (persistence.ClosedExecutionStore, error)

	mu    sync.Mutex
	store persistence.ClosedExecutionStore
}

// NewLazyClosedStore defers open until the first call. A failed open is retried on the next call.
func NewLazyClosedStore(open func() (persistence.ClosedExecutionStore, error)) persistence.ClosedExecutionStore {
	return &lazyClosedStore{open: open}
}

func (l *lazyClosedStore) get() (persistence.ClosedExecutionStore, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store == nil {
		store, err := l.open()
		if err != nil {
			return nil, err
		}
		l.store = store
	}
	return l.store, nil
}

func (l *lazyClosedStore) GetName() string { return "warm-storage-lazy" }

func (l *lazyClosedStore) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store != nil {
		l.store.Close()
	}
}

func (l *lazyClosedStore) InsertClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey, data *persistence.DataBlob) error {
	store, err := l.get()
	if err != nil {
		return err
	}
	return store.InsertClosedExecution(ctx, key, data)
}

func (l *lazyClosedStore) GetClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) (*persistence.DataBlob, error) {
	store, err := l.get()
	if err != nil {
		return nil, err
	}
	return store.GetClosedExecution(ctx, key)
}

func (l *lazyClosedStore) DeleteClosedExecution(ctx context.Context, key persistence.ClosedExecutionKey) error {
	store, err := l.get()
	if err != nil {
		return err
	}
	return store.DeleteClosedExecution(ctx, key)
}
