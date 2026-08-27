package archivecontextdetach

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

// TestArchiveReadHonorsCanceledContext verifies that archive reads stop when
// the request lifecycle is cancelled before the storage call begins.
func TestArchiveReadHonorsCanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := json.Marshal(domain.ReleaseArchive{
		ArchiveID: "arc-case-1", CaseID: "case-1", Decision: "release", WitnessID: "见证人",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO archives(case_id,archive_id,manifest_digest,payload,metadata,created_at) VALUES(?,?,?,?,?,datetime('now'))`,
		"case-1", "arc-case-1", "manifest", []byte("{}"), meta); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Archive(ctx, "case-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("TestArchiveReadHonorsCanceledContext: expected context cancellation, got %v", err)
	}
}
