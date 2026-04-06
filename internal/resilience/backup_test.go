package resilience

import (
	"os"
	"testing"
)

func TestBackupManager_CreateAndList(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackupManager(dir)

	snap, err := bm.CreateSnapshot()
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.ID == "" {
		t.Fatal("snapshot ID should not be empty")
	}
	if snap.Size == 0 {
		t.Fatal("snapshot size should not be 0")
	}
	if snap.Hash == "" {
		t.Fatal("snapshot hash should not be empty")
	}
	if len(snap.Components) == 0 {
		t.Fatal("snapshot should include components")
	}

	snaps := bm.ListSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].ID != snap.ID {
		t.Fatalf("expected ID %s, got %s", snap.ID, snaps[0].ID)
	}
}

func TestBackupManager_Verify(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackupManager(dir)

	snap, err := bm.CreateSnapshot()
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	if err := bm.VerifySnapshot(snap.ID); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}
}

func TestBackupManager_VerifyCorrupted(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackupManager(dir)

	snap, err := bm.CreateSnapshot()
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	// Corrupt the file.
	filename := dir + "/snapshots/" + snap.ID + ".json"
	if err := os.WriteFile(filename, []byte("not json"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	if err := bm.VerifySnapshot(snap.ID); err == nil {
		t.Fatal("verify should fail on corrupted snapshot")
	}
}

func TestBackupManager_RestoreNotFound(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackupManager(dir)

	if err := bm.RestoreSnapshot("nonexistent"); err == nil {
		t.Fatal("restore should fail for missing snapshot")
	}
}

func TestBackupManager_Restore(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackupManager(dir)

	snap, err := bm.CreateSnapshot()
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if err := bm.RestoreSnapshot(snap.ID); err != nil {
		t.Fatalf("restore should succeed: %v", err)
	}
}
