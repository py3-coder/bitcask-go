package storage

import (
	"bytes"
	"fmt"
	"testing"
)

func openTestDB(t *testing.T) *Bitcask {
	t.Helper()

	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	return db
}

// writeLargeValue writes a value large enough to force segment rotation
// when multiple keys are inserted.
func writeLargeValue(t *testing.T, db *Bitcask, key string) {
	t.Helper()

	value := bytes.Repeat([]byte("x"), 1024*1024)

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Put(%q) failed: %v", key, err)
	}
}

func assertValue(t *testing.T, db *Bitcask, key string, want []byte) {
	t.Helper()

	got, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get(%q) failed: %v", key, err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("Get(%q) returned an unexpected value: got %d bytes, want %d bytes",
			key, len(got), len(want))
	}
}

func TestCompactionKeepsLatestValue(t *testing.T) {
	db := openTestDB(t)

	writeLargeValue(t, db, "key-0")
	writeLargeValue(t, db, "key-1")

	latestValue := []byte("latest-value")
	if err := db.Put("key-0", latestValue); err != nil {
		t.Fatalf("failed to overwrite key-0: %v", err)
	}

	if err := db.Compaction(); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	assertValue(t, db, "key-0", latestValue)
	assertValue(t, db, "key-1", bytes.Repeat([]byte("x"), 1024*1024))
}

func TestCompactionWithMultipleSegments(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 5; i++ {
		writeLargeValue(t, db, fmt.Sprintf("key-%d", i))
	}

	latestValue := []byte("latest-value")
	if err := db.Put("key-0", latestValue); err != nil {
		t.Fatalf("failed to overwrite key-0: %v", err)
	}

	if err := db.Compaction(); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)

		if i == 0 {
			assertValue(t, db, key, latestValue)
		} else {
			assertValue(t, db, key, bytes.Repeat([]byte("x"), 1024*1024))
		}
	}
}

func TestCompactionCanRunTwice(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 5; i++ {
		writeLargeValue(t, db, fmt.Sprintf("key-%d", i))
	}

	latestValue := []byte("latest-value")
	if err := db.Put("key-0", latestValue); err != nil {
		t.Fatalf("failed to overwrite key-0: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := db.Compaction(); err != nil {
			t.Fatalf("compaction run %d failed: %v", i+1, err)
		}
	}

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)

		if i == 0 {
			assertValue(t, db, key, latestValue)
		} else {
			assertValue(t, db, key, bytes.Repeat([]byte("x"), 1024*1024))
		}
	}
}

func TestCompactionSurvivesReopen(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	for i := 0; i < 5; i++ {
		writeLargeValue(t, db, fmt.Sprintf("key-%d", i))
	}

	latestValue := []byte("latest-value")
	if err := db.Put("key-0", latestValue); err != nil {
		t.Fatalf("failed to overwrite key-0: %v", err)
	}

	if err := db.Compaction(); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	// Reopen the same directory to test index reconstruction from disk.
	db, err = Open(dir)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close reopened database: %v", err)
		}
	}()

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)

		if i == 0 {
			assertValue(t, db, key, latestValue)
		} else {
			assertValue(t, db, key, bytes.Repeat([]byte("x"), 1024*1024))
		}
	}
}
func TestCompactionRemovesDeletedKeys(t *testing.T) {
	db := openTestDB(t)

	writeLargeValue(t, db, "key-1")

	if err := db.Delete("key-1"); err != nil {
		t.Fatal(err)
	}

	// Force rotation by writing another large record.
	writeLargeValue(t, db, "key-2")

	if err := db.Compaction(); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	if _, exists := db.index["key-1"]; exists {
		t.Fatal("deleted key still exists in index")
	}

	if _, err := db.Get("key-1"); err == nil {
		t.Fatal("expected deleted key to be absent")
	}
}

func TestCompactionDeletedKeysPersistAfterReopen(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	writeLargeValue(t, db, "key-1")

	if err := db.Delete("key-1"); err != nil {
		t.Fatal(err)
	}

	writeLargeValue(t, db, "key-2")

	if err := db.Compaction(); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Get("key-1"); err == nil {
		t.Fatal("deleted key reappeared after reopen")
	}
}

func TestCompactionPreservesNonDeletedKeys(t *testing.T) {
	db := openTestDB(t)

	writeLargeValue(t, db, "deleted")

	if err := db.Delete("deleted"); err != nil {
		t.Fatal(err)
	}

	// This write triggers rotation.
	writeLargeValue(t, db, "alive")

	if err := db.Compaction(); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	if _, err := db.Get("deleted"); err == nil {
		t.Fatal("deleted key should not exist")
	}

	// alive is in the active segment and must survive compaction.
	assertValue(t, db, "alive", bytes.Repeat([]byte("x"), 1024*1024))
}
func TestPutAfterCompactionSurvivesReopen(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	writeLargeValue(t, db, "key-1")
	writeLargeValue(t, db, "key-2")

	if err := db.Compaction(); err != nil {
		t.Fatal(err)
	}

	// Write after compaction
	if err := db.Put("key-3", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	assertValue(t, db, "key-1", bytes.Repeat([]byte("x"), 1024*1024))
	assertValue(t, db, "key-2", bytes.Repeat([]byte("x"), 1024*1024))
	assertValue(t, db, "key-3", []byte("hello"))
}
