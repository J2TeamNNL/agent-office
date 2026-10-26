package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/tung1998/agent-office/migrations"
)

// In test binaries every test opens a fresh database, and running all the
// migrations each time made the suite slow. The first test migrates a template
// once (kept in the temp dir, keyed by the migrations' content, so the next
// runs reuse it) and new databases start as a copy of it.
var (
	templateOnce sync.Once
	templatePath string
)

// fromTemplate replaces a still-empty test database with a copy of the
// migrated template. False means the caller migrates as usual.
func (s *Store) fromTemplate(ctx context.Context) bool {
	if s.path == ":memory:" || s.path == "" {
		return false
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master`).Scan(&n); err != nil || n > 0 {
		return false
	}
	templateOnce.Do(func() { templatePath = buildTemplate(ctx) })
	if templatePath == "" {
		return false
	}
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return false
	}
	s.db.Close()
	os.Remove(s.path + "-wal")
	os.Remove(s.path + "-shm")
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		panic(err) // the store is closed; nothing sane to go on with
	}
	db, err := openDB(s.path)
	if err != nil {
		panic(err)
	}
	s.db, s.q = db, db
	return true
}

func buildTemplate(ctx context.Context) string {
	h := sha256.New()
	fs.WalkDir(migrations.SQLite, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := migrations.SQLite.ReadFile(p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	path := filepath.Join(os.TempDir(), "agent-office-testdb-"+hex.EncodeToString(h.Sum(nil))[:16]+".db")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	dir, err := os.MkdirTemp("", "agent-office-testdb-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	tmp := filepath.Join(dir, "t.db")
	st, err := Open(tmp)
	if err != nil {
		return ""
	}
	st.path = "" // migrate for real
	err = st.Migrate(ctx)
	if err == nil {
		_, err = st.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	st.Close()
	if err != nil {
		return ""
	}
	// Packages test in parallel: rename is atomic, the last writer wins with
	// the same content.
	if err := os.Rename(tmp, path); err != nil {
		return ""
	}
	return path
}
