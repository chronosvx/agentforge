package session_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/session"
)

func TestSyncToRemoteUploadsAll(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	base := time.Unix(1000, 0)
	mustSave(t, store, session.Entry{ID: "b", Title: "B", UpdatedAt: base.Add(time.Second)})
	mustSave(t, store, session.Entry{ID: "a", Title: "A", UpdatedAt: base})

	remote := newMapRemote()
	ids, err := session.SyncToRemote(context.Background(), dataDir, remote)
	if err != nil {
		t.Fatalf("sync to remote: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected uploaded ids: %v", ids)
	}

	for _, id := range []string{"a", "b"} {
		data, err := remote.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("remote get %s: %v", id, err)
		}
		var entry session.Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatalf("decode remote %s: %v", id, err)
		}
		if entry.ID != id {
			t.Fatalf("remote %s id = %q", id, entry.ID)
		}
	}
}

func TestSyncFromRemoteDownloadsAndSkipsNewerLocal(t *testing.T) {
	remote := newMapRemote()
	base := time.Unix(2000, 0)

	putRemoteEntry(t, remote, session.Entry{ID: "new", Title: "remote-new", UpdatedAt: base})
	putRemoteEntry(t, remote, session.Entry{ID: "older", Title: "remote-older", UpdatedAt: base.Add(time.Second)})
	putRemoteEntry(t, remote, session.Entry{ID: "newer", Title: "remote-newer", UpdatedAt: base.Add(3 * time.Second)})

	dataDir := t.TempDir()
	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// Local "older" is newer than remote -> should be skipped.
	mustSave(t, store, session.Entry{ID: "older", Title: "local-newer", UpdatedAt: base.Add(2 * time.Second)})
	// Local "newer" is older than remote -> should be overwritten.
	mustSave(t, store, session.Entry{ID: "newer", Title: "local-older", UpdatedAt: base.Add(time.Second)})

	ids, err := session.SyncFromRemote(context.Background(), dataDir, remote)
	if err != nil {
		t.Fatalf("sync from remote: %v", err)
	}
	if len(ids) != 2 || ids[0] != "new" || ids[1] != "newer" {
		t.Fatalf("unexpected downloaded ids: %v", ids)
	}

	assertTitle(t, store, "new", "remote-new")
	assertTitle(t, store, "older", "local-newer")
	assertTitle(t, store, "newer", "remote-newer")
}

func TestSyncFromRemoteSkipsEqualTimestamp(t *testing.T) {
	remote := newMapRemote()
	base := time.Unix(3000, 0)
	putRemoteEntry(t, remote, session.Entry{ID: "same", Title: "remote", UpdatedAt: base})

	dataDir := t.TempDir()
	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	mustSave(t, store, session.Entry{ID: "same", Title: "local", UpdatedAt: base})

	ids, err := session.SyncFromRemote(context.Background(), dataDir, remote)
	if err != nil {
		t.Fatalf("sync from remote: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no downloads, got %v", ids)
	}
	assertTitle(t, store, "same", "local")
}

func TestSyncFromRemoteCreatesDataDir(t *testing.T) {
	remote := newMapRemote()
	putRemoteEntry(t, remote, session.Entry{ID: "s1", Title: "from remote", UpdatedAt: time.Unix(10, 0)})

	dataDir := t.TempDir() // NewStore not called; sessions dir may not exist yet.
	ids, err := session.SyncFromRemote(context.Background(), dataDir, remote)
	if err != nil {
		t.Fatalf("sync from remote: %v", err)
	}
	if len(ids) != 1 || ids[0] != "s1" {
		t.Fatalf("unexpected ids: %v", ids)
	}

	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store after sync: %v", err)
	}
	assertTitle(t, store, "s1", "from remote")
}

func putRemoteEntry(t *testing.T, remote *mapRemote, entry session.Entry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal remote entry: %v", err)
	}
	if err := remote.Put(context.Background(), entry.ID, data); err != nil {
		t.Fatalf("put remote entry: %v", err)
	}
}
