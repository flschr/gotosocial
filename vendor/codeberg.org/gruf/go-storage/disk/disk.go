package disk

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"

	"codeberg.org/gruf/go-byteutil"
	"codeberg.org/gruf/go-storage"
	"codeberg.org/gruf/go-storage/internal"
)

// ensure DiskStorage conforms to storage.Storage.
var _ storage.Storage = (*DiskStorage)(nil)

// DefaultConfig returns the default DiskStorage configuration.
func DefaultConfig() Config {
	return defaultConfig
}

// immutable default configuration.
var defaultConfig = Config{
	Create:     OpenArgs{syscall.O_CREAT | syscall.O_WRONLY, 0o644},
	MkdirPerms: 0o755,
	CopyFn:     io.Copy,
}

// OpenArgs defines args passed
// in a syscall.Open() operation.
type OpenArgs struct {
	Flags int
	Perms uint32
}

// Config defines options to be
// used when opening a DiskStorage.
type Config struct {

	// Create are the arguments passed
	// to syscall.Open() when creating
	// a file for write operations.
	Create OpenArgs

	// MkdirPerms are the permissions used
	// when creating necessary sub-dirs in
	// a storage key with slashes.
	MkdirPerms uint32

	// CopyFn allows specifying an alternative to
	// io.Copy() to use, e.g. if you would like to
	// provide pooled buffers, or custom buffer sizes.
	CopyFn func(io.Writer, io.Reader) (int64, error)
}

// getDiskConfig returns valid (and owned!) Config for given ptr.
func getDiskConfig(cfg *Config) Config {
	if cfg == nil {
		// use defaults.
		return defaultConfig
	}

	// Ensure non-zero syscall args.
	if cfg.Create.Flags == 0 {
		cfg.Create.Flags = defaultConfig.Create.Flags
	}
	if cfg.Create.Perms == 0 {
		cfg.Create.Perms = defaultConfig.Create.Perms
	}
	if cfg.MkdirPerms == 0 {
		cfg.MkdirPerms = defaultConfig.MkdirPerms
	}

	return Config{
		Create:     cfg.Create,
		MkdirPerms: cfg.MkdirPerms,
		CopyFn:     cfg.CopyFn,
	}
}

// DiskStorage is a Storage implementation
// that stores directly to a filesystem.
type DiskStorage struct {
	Config
	FS
}

// Open opens a DiskStorage instance for given folder path and configuration.
func Open(path string, cfg *Config) (*DiskStorage, error) {

	// Check + set config defaults.
	config := getDiskConfig(cfg)

	// Clean provided storage path, ensure
	// final '/' to help with path trimming.
	pb := internal.GetPathBuilder()
	path = pb.Clean(path) + "/"
	internal.PutPathBuilder(pb)

	// Ensure directories up-to path exist.
	perms := fs.FileMode(config.MkdirPerms)
	err := os.MkdirAll(path, perms)
	if err != nil {
		return nil, err
	}

	// Prepare DiskStorage.
	st := &DiskStorage{
		Config: config,
		FS:     FS{path},
	}

	return st, nil
}

// Clean: implements Storage.Clean().
func (st *DiskStorage) Clean(_ context.Context) error {
	return clean_dirs(st.FS.base)
}

// ReadBytes: implements Storage.ReadBytes().
func (st *DiskStorage) ReadBytes(ctx context.Context, key string) ([]byte, error) {

	// Get stream reader for key.
	rc, err := st.ReadStream(ctx, key)
	if err != nil {
		return nil, err
	}

	// Read all data to memory.
	data, err := io.ReadAll(rc)

	// Close the reader.
	err2 := rc.Close()

	if err != nil {
		return nil, err
	} else if err2 != nil {
		return nil, err2
	}

	return data, nil
}

// ReadStream: implements Storage.ReadStream().
func (st *DiskStorage) ReadStream(_ context.Context, key string) (io.ReadCloser, error) {
	return st.Open(key, readArgs)
}

// WriteBytes: implements Storage.WriteBytes().
func (st *DiskStorage) WriteBytes(ctx context.Context, key string, value []byte) (int, error) {
	n, err := st.WriteStream(ctx, key, bytes.NewReader(value))
	return int(n), err
}

// WriteStream: implements Storage.WriteStream().
func (st *DiskStorage) WriteStream(_ context.Context, key string, stream io.Reader) (int64, error) {

	// Acquire path builder buffer.
	pb := internal.GetPathBuilder()

	// Generate the file path for key.
	kpath, err := st.filepath(pb, key)

	// Done with path buffer.
	internal.PutPathBuilder(pb)

	if err != nil {
		return 0, err
	}

	// Fast check for whether this may be a
	// sub-directory. This is not a definitive
	// check, but it indicates to try MkdirAll.
	if strings.ContainsRune(key, '/') {

		// Get dir of key path.
		dir := path.Dir(kpath)

		// Ensure required key path dirs exist.
		perms := fs.FileMode(st.MkdirPerms)
		err = os.MkdirAll(dir, perms)
		if err != nil {
			return 0, err
		}
	}

	// Attempt to open with create args.
	file, err := open(kpath, st.Create)
	switch err {
	case nil:

	case syscall.EEXIST:
		if st.Create.Flags&syscall.O_EXCL != 0 {
			// Translate already exists errors and wrap with the key path.
			return 0, internal.ErrWithKey(storage.ErrAlreadyExists, kpath)
		}

	default:
		return 0, err
	}

	// Ensure file closed.
	defer file.Close()

	if st.CopyFn != nil {
		// Use provided io copy function.
		return st.CopyFn(file, stream)
	} else {
		// Use default io.Copy func.
		return io.Copy(file, stream)
	}
}

// Stat implements Storage.Stat().
func (st *DiskStorage) Stat(_ context.Context, key string) (*storage.Entry, error) {
	stat, err := st.FS.Stat(key)
	if err != nil {
		if internal.Is(err, storage.ErrNotFound) {
			err = nil // mask not-found errors
		}
		return nil, err
	}
	return &storage.Entry{
		Modified: modtime(stat),
		Size:     stat.Size,
		Key:      key,
	}, nil
}

// Remove implements Storage.Remove().
func (st *DiskStorage) Remove(_ context.Context, key string) error {
	return st.Unlink(key)
}

// WalkKeys implements Storage.WalkKeys().
func (st *DiskStorage) WalkKeys(_ context.Context, opts storage.WalkKeysOpts) error {
	if opts.Step == nil {
		panic("nil step fn")
	}

	// Acquire path builder buffer.
	pb := internal.GetPathBuilder()
	defer internal.PutPathBuilder(pb)

	// Directory to walk.
	dir := st.FS.base

	if opts.Prefix != "" {
		// Convert key prefix to one of our filepaths.
		pathprefix, err := st.filepath(pb, opts.Prefix)
		if err != nil {
			return internal.ErrWithMsg(err, "prefix error")
		}

		// Fast check for whether this may be a
		// sub-directory. This is not a definitive
		// check, but it allows us to update the
		// directory we walk to narrow search params.
		if strings.ContainsRune(opts.Prefix, '/') {
			dir = path.Dir(pathprefix)
		}

		// Set updated storage
		// path prefix in opts.
		opts.Prefix = pathprefix
	}

	// Reusable sys stat model.
	var stat_t syscall.Stat_t

	return walk_dir(pb, dir, func(absdir, reldir string, ent *Dirent) error {
		if !ent.IsRegular() {
			// Ignore anything but
			// regular file types.
			return nil
		}

		// Get a temp. copy of entry name.
		name := byteutil.B2S(ent.nameptr())

		// Generate relative path.
		rel := pb.Join(reldir, name)

		// Perform a fast filter check against storage path prefix (if set).
		if opts.Prefix != "" && !strings.HasPrefix(rel, opts.Prefix) {
			return nil // ignore
		}

		// Ignore filtered keys.
		if opts.Filter != nil &&
			!opts.Filter(rel) {
			return nil // ignore
		}

		// Generate absolute path.
		abs := pb.Join(absdir, name)

		// Stat file info at path.
		err := lstat(abs, &stat_t)
		switch err {
		case nil:

		// Race condition, it
		// was deleted after the
		// initial readdir() call.
		case syscall.ENOENT:
			return nil

		default:
			return err
		}

		// Perform provided walk function
		return opts.Step(storage.Entry{
			Modified: modtime(stat_t),
			Size:     stat_t.Size,
			Key:      rel,
		})
	})
}
