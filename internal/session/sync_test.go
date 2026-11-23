package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/session"
)

func TestExportAll(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	entry := session.Entry{
		ID:        "s1",
		Title:     "hello",
		UpdatedAt: time.Unix(100, 0),
	}
	if err := store.Save(entry); err != nil {
		t.Fatalf("save: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "exports")
	paths, err := session.ExportAll(dataDir, outDir)
	if err != nil {
		t.Fatalf("export all: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 exported file, got %d: %v", len(paths), paths)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "s1.json"))
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	var got session.Entry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode exported file: %v", err)
	}
	if got.ID != entry.ID || got.Title != entry.Title {
		t.Fatalf("unexpected exported entry: %+v", got)
	}
}

func TestImportAllSkipsExisting(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save(session.Entry{ID: "existing", Title: "old"}); err != nil {
		t.Fatalf("save existing: %v", err)
	}

	inDir := t.TempDir()
	writeTestJSON(t, filepath.Join(inDir, "new.json"), session.Entry{ID: "new", Title: "new title"})
	writeTestJSON(t, filepath.Join(inDir, "existing.json"), session.Entry{ID: "existing", Title: "should not overwrite"})

	results, err := session.ImportAll(inDir, dataDir)
	if err != nil {
		t.Fatalf("import all: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}

	got, err := store.Load("new")
	if err != nil {
		t.Fatalf("load imported new: %v", err)
	}
	if got.Title != "new title" {
		t.Fatalf("unexpected imported title: %q", got.Title)
	}

	gotExisting, err := store.Load("existing")
	if err != nil {
		t.Fatalf("load existing: %v", err)
	}
	if gotExisting.Title != "old" {
		t.Fatalf("existing session was overwritten: %q", gotExisting.Title)
	}
}

func TestSyncDirBidirectional(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcStore, err := session.NewStore(srcDir)
	if err != nil {
		t.Fatalf("new src store: %v", err)
	}
	dstStore, err := session.NewStore(dstDir)
	if err != nil {
		t.Fatalf("new dst store: %v", err)
	}

	base := time.Unix(1000, 0)
	mustSave(t, srcStore, session.Entry{ID: "a", Title: "src-new", UpdatedAt: base.Add(2 * time.Second)})
	mustSave(t, dstStore, session.Entry{ID: "a", Title: "dst-old", UpdatedAt: base})
	mustSave(t, srcStore, session.Entry{ID: "b", Title: "only-src", UpdatedAt: base.Add(time.Second)})
	mustSave(t, dstStore, session.Entry{ID: "c", Title: "only-dst", UpdatedAt: base.Add(time.Second)})
	mustSave(t, dstStore, session.Entry{ID: "d", Title: "dst-new", UpdatedAt: base.Add(2 * time.Second)})
	mustSave(t, srcStore, session.Entry{ID: "d", Title: "src-old", UpdatedAt: base})

	ops, err := session.SyncDir(srcDir, dstDir)
	if err != nil {
		t.Fatalf("sync dir: %v", err)
	}

	wantOps := map[string]string{
		"a": "src->dst",
		"b": "src->dst",
		"c": "dst->src",
		"d": "dst->src",
	}
	if len(ops) != len(wantOps) {
		t.Fatalf("unexpected ops: %v", ops)
	}
	for id, want := range wantOps {
		if got := ops[id]; got != want {
			t.Fatalf("op for %s = %q, want %q", id, got, want)
		}
	}

	assertTitle(t, dstStore, "a", "src-new")
	assertTitle(t, dstStore, "b", "only-src")
	assertTitle(t, srcStore, "c", "only-dst")
	assertTitle(t, srcStore, "d", "dst-new")
}

func TestSyncDirAcceptsSessionsDirDirectly(t *testing.T) {
	srcData := t.TempDir()
	dstData := t.TempDir()
	srcStore, err := session.NewStore(srcData)
	if err != nil {
		t.Fatalf("new src store: %v", err)
	}
	dstStore, err := session.NewStore(dstData)
	if err != nil {
		t.Fatalf("new dst store: %v", err)
	}
	mustSave(t, srcStore, session.Entry{ID: "x", Title: "from-src", UpdatedAt: time.Unix(10, 0)})

	ops, err := session.SyncDir(filepath.Join(srcData, "sessions"), filepath.Join(dstData, "sessions"))
	if err != nil {
		t.Fatalf("sync direct sessions dirs: %v", err)
	}
	if ops["x"] != "src->dst" {
		t.Fatalf("unexpected ops: %v", ops)
	}
	assertTitle(t, dstStore, "x", "from-src")
}

func TestExportImportEncryptedAndMixedSessions(t *testing.T) {
	dataDir := t.TempDir()
	encStore, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}
	if err := encStore.Save(session.Entry{ID: "enc1", Title: "encrypted", UpdatedAt: time.Unix(200, 0)}); err != nil {
		t.Fatalf("save encrypted: %v", err)
	}
	// 同一 ID 再写一份明文副本，以及一条纯明文会话。
	writeTestJSON(t, filepath.Join(dataDir, "sessions", "enc1.json"), session.Entry{ID: "enc1", Title: "duplicate plaintext"})
	writeTestJSON(t, filepath.Join(dataDir, "sessions", "plain1.json"), session.Entry{ID: "plain1", Title: "plaintext"})

	outDir := filepath.Join(t.TempDir(), "exports")
	report, err := session.ExportAllWithReport(dataDir, outDir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if report.Encrypted != 1 || report.Plaintext != 1 {
		t.Fatalf("export report = %+v, want 1 encrypted + 1 plaintext", report)
	}
	if len(report.Skipped) != 1 {
		t.Fatalf("export skipped = %v, want one duplicate", report.Skipped)
	}
	if _, err := os.Stat(filepath.Join(outDir, "enc1.json.enc")); err != nil {
		t.Fatalf("encrypted export missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "plain1.json")); err != nil {
		t.Fatalf("plaintext export missing: %v", err)
	}

	targetDir := t.TempDir()
	importReport, err := session.ImportAllWithReport(outDir, targetDir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(importReport.Imported) != 2 || len(importReport.Failed) != 0 {
		t.Fatalf("import report = %+v, want 2 imported", importReport)
	}
	targetEnc, err := session.NewEncryptedStore(targetDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new target encrypted store: %v", err)
	}
	got, err := targetEnc.Load("enc1")
	if err != nil {
		t.Fatalf("load imported encrypted session: %v", err)
	}
	if got.Title != "encrypted" {
		t.Fatalf("imported encrypted title = %q, want encrypted", got.Title)
	}

	// 目标已有加密文件时，再导入同 ID 明文应跳过。
	dupDir := t.TempDir()
	writeTestJSON(t, filepath.Join(dupDir, "enc1.json"), session.Entry{ID: "enc1", Title: "should not overwrite"})
	dupReport, err := session.ImportAllWithReport(dupDir, targetDir)
	if err != nil {
		t.Fatalf("duplicate import: %v", err)
	}
	if len(dupReport.Imported) != 0 || len(dupReport.Skipped) != 1 {
		t.Fatalf("duplicate import report = %+v, want one skipped", dupReport)
	}
}

func writeTestJSON(t *testing.T, path string, entry session.Entry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSave(t *testing.T, store *session.Store, entry session.Entry) {
	t.Helper()
	if err := store.Save(entry); err != nil {
		t.Fatalf("save %s: %v", entry.ID, err)
	}
}

func assertTitle(t *testing.T, store *session.Store, id, want string) {
	t.Helper()
	entry, err := store.Load(id)
	if err != nil {
		t.Fatalf("load %s: %v", id, err)
	}
	if entry.Title != want {
		t.Fatalf("%s title = %q, want %q", id, entry.Title, want)
	}
}
