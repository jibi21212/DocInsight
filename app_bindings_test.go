package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/store"
)

// newTestApp builds an App backed by a fresh temp SQLite store with the local
// user provisioned — enough to exercise the store-backed binding methods without
// the sidecar/worker pool.
func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(db.Close)
	a := &App{ctx: context.Background(), cfg: config.Load(), store: db}
	if err := a.ensureLocalUser(); err != nil {
		t.Fatalf("ensureLocalUser: %v", err)
	}
	if a.userID == nil {
		t.Fatal("local user not set")
	}
	return a
}

// TestBindings_StoreBackedRoundTrips exercises the binding methods that only need
// the store, proving the Phase-2 App layer is wired to the (already-tested) store
// and scoped to the local user.
func TestBindings_StoreBackedRoundTrips(t *testing.T) {
	a := newTestApp(t)

	if f, err := a.CreateFolder("Research", ""); err != nil || f == nil {
		t.Fatalf("CreateFolder: f=%v err=%v", f, err)
	}
	folders, err := a.ListFolders("")
	if err != nil || len(folders) != 1 {
		t.Fatalf("ListFolders: n=%d err=%v", len(folders), err)
	}

	if _, err := a.CreateTag("important", "#ef4444"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tags, err := a.ListTags(); err != nil || len(tags) != 1 {
		t.Fatalf("ListTags: n=%d err=%v", len(tags), err)
	}

	page, err := a.ListDocuments(1, 20, "")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("expected empty library, got total=%d len=%d", page.Total, len(page.Data))
	}

	if _, err := a.CreateAgentSession("anthropic", "claude-sonnet-4-6", "Chat", ""); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	if sessions, err := a.ListAgentSessions(); err != nil || len(sessions) != 1 {
		t.Fatalf("ListAgentSessions: n=%d err=%v", len(sessions), err)
	}

	// Bad ID should error, not panic.
	if err := a.DeleteDocument("not-a-uuid"); err == nil {
		t.Fatal("DeleteDocument with bad id should error")
	}
}
