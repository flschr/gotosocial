package memory

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"codeberg.org/gruf/go-storage"

	"codeberg.org/gruf/go-storage/internal"
)

// ensure MemoryStorage conforms to storage.Storage.
var _ storage.Storage = (*MemoryStorage)(nil)

// MemoryStorage is a storage implementation that simply stores key-value
// pairs in a Go map in-memory. The map is protected by a mutex.
type MemoryStorage struct {
	ow bool // overwrites
	fs map[string]file
	mu sync.Mutex
}

// file wraps file data
// with last-mod time.
type file struct {
	data []byte
	mtim time.Time
}

// Open opens a new MemoryStorage instance with internal map starting size.
func Open(size int, overwrites bool) *MemoryStorage {
	return &MemoryStorage{
		fs: make(map[string]file, size),
		ow: overwrites,
	}
}

// Clean: implements Storage.Clean().
func (st *MemoryStorage) Clean(_ context.Context) error {

	// Lock map.
	st.mu.Lock()

	// Copy old.
	old := st.fs

	// Resize map to only necessary size in-mem.
	st.fs = make(map[string]file, len(st.fs))
	for key, val := range old {
		st.fs[key] = val
	}

	// Done with lock.
	st.mu.Unlock()

	return nil
}

// ReadBytes: implements Storage.ReadBytes().
func (st *MemoryStorage) ReadBytes(_ context.Context, key string) ([]byte, error) {
	var b []byte

	// Lock map.
	st.mu.Lock()

	// Check key in store.
	file, ok := st.fs[key]
	if ok {

		// COPY file bytes.
		b = copyb(file.data)
	}

	// Done with lock.
	st.mu.Unlock()

	if !ok {
		return nil, internal.ErrWithKey(storage.ErrNotFound, key)
	}

	return b, nil
}

// ReadStream: implements Storage.ReadStream().
func (st *MemoryStorage) ReadStream(ctx context.Context, key string) (io.ReadCloser, error) {

	// Read value data from store.
	b, err := st.ReadBytes(ctx, key)
	if err != nil {
		return nil, err
	}

	// Wrap in readcloser.
	r := bytes.NewReader(b)
	return io.NopCloser(r), nil
}

// WriteBytes: implements Storage.WriteBytes().
func (st *MemoryStorage) WriteBytes(_ context.Context, key string, b []byte) (int, error) {

	// Lock map.
	st.mu.Lock()

	// Check key in store.
	_, ok := st.fs[key]

	if ok && !st.ow {
		// Done with lock.
		st.mu.Unlock()

		// Overwrites are disabled, return an existing key error.
		return 0, internal.ErrWithKey(storage.ErrAlreadyExists, key)
	}

	// Write copy to store.
	st.fs[key] = file{
		mtim: time.Now(),
		data: copyb(b),
	}

	// Done with lock.
	st.mu.Unlock()

	return len(b), nil
}

// WriteStream: implements Storage.WriteStream().
func (st *MemoryStorage) WriteStream(ctx context.Context, key string, r io.Reader) (int64, error) {

	// Read all from reader.
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}

	// Write in-memory data to store.
	n, err := st.WriteBytes(ctx, key, b)
	return int64(n), err
}

// Stat: implements Storage.Stat().
func (st *MemoryStorage) Stat(_ context.Context, key string) (*storage.Entry, error) {

	// Lock map.
	st.mu.Lock()

	// Check key in store.
	file, ok := st.fs[key]

	// Get file entry size.
	sz := int64(len(file.data))

	// Done with lock.
	st.mu.Unlock()

	if !ok {
		return nil, nil
	}

	return &storage.Entry{
		Modified: file.mtim,
		Size:     sz,
		Key:      key,
	}, nil
}

// Remove: implements Storage.Remove().
func (st *MemoryStorage) Remove(_ context.Context, key string) error {

	// Lock map.
	st.mu.Lock()

	// Check key in store.
	_, ok := st.fs[key]

	if ok {
		// Delete store key.
		delete(st.fs, key)
	}

	// Done with lock.
	st.mu.Unlock()

	if !ok {
		return internal.ErrWithKey(storage.ErrNotFound, key)
	}

	return nil
}

// WalkKeys: implements Storage.WalkKeys().
func (st *MemoryStorage) WalkKeys(_ context.Context, opts storage.WalkKeysOpts) error {
	if opts.Step == nil {
		panic("nil step fn")
	}

	// Extract filter func.
	filter := opts.Filter

	switch {
	case opts.Prefix == "":
		// nothing to update.

	case filter != nil:
		// Filter according to prefix
		// BEFORE passing to filter func.
		filter = func(key string) bool {
			if strings.HasPrefix(key, opts.Prefix) {
				return false
			}
			return opts.Filter(key)
		}

	default: // filter == nil
		// Filter according to prefix.
		filter = func(key string) bool {
			return strings.HasPrefix(key, opts.Prefix)
		}
	}

	var err error

	// Lock map.
	st.mu.Lock()

	// Ensure unlocked.
	defer st.mu.Unlock()

	// Range key-vals in hash map.
	//
	// Use different loops depending
	// on if filter func was provided,
	// to reduce loop operations.
	if filter != nil {
		for key, val := range st.fs {
			// Check filtering.
			if !filter(key) {
				continue
			}

			// Pass to provided step func.
			err = opts.Step(storage.Entry{
				Modified: val.mtim,
				Size:     int64(len(val.data)),
				Key:      key,
			})
			if err != nil {
				return err
			}
		}
	} else {
		for key, val := range st.fs {
			// Pass to provided step func.
			err = opts.Step(storage.Entry{
				Modified: val.mtim,
				Size:     int64(len(val.data)),
				Key:      key,
			})
			if err != nil {
				return err
			}
		}
	}

	return err
}

// copyb returns a copy of byte-slice b.
func copyb(b []byte) []byte {
	if b == nil {
		return nil
	}
	p := make([]byte, len(b))
	_ = copy(p, b)
	return p
}
