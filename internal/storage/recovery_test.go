package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryTruncatesIncompleteTail(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Put("name", []byte("Saurabh")); err != nil {
		t.Fatal(err)
	}
	if err := db.Put("language", []byte("Go")); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "000001.data")

	// Simulate a crash leaving a partial header at EOF.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening should discard the incomplete tail.
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	got, err := db.Get("name")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Saurabh" {
		t.Fatalf("got %q, want %q", got, "Saurabh")
	}

	if _, err := db.Get("language"); err != nil {
		t.Fatalf("valid record was lost: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(19+18) {
		t.Fatalf("file size = %d, want 37", info.Size())
	}
}

func TestRecoveryTruncatesIncompleteRecordBody(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Put("name", []byte("Saurabh")); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "000001.data")

	// Header claims key length 4 and value length 7,
	// but only 3 body bytes follow.
	partial := []byte{
		0, 0, 0, 4,
		0, 0, 0, 7,
		'n', 'a', 'm',
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.Write(partial); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	got, err := db.Get("name")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Saurabh" {
		t.Fatalf("got %q, want %q", got, "Saurabh")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 19 {
		t.Fatalf("file size = %d, want 19", info.Size())
	}
}

func TestRecoveryRejectsCorruptionInMiddle(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Put("first", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := db.Put("second", []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := db.Put("third", []byte("three")); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "000001.data")

	// First record size = 8 + len("first") + len("one") = 16.
	// Corrupt the second record's header at offset 16.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Set the second record's key length to an impossible value.
	corruptHeader := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	if _, err := f.WriteAt(corruptHeader, 16); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	db, err = Open(dir)
	if err == nil {
		db.Close()
		t.Fatal("expected recovery error for corrupted record")
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if after.Size() != before.Size() {
		t.Fatalf(
			"recovery modified corrupted file: size before=%d after=%d",
			before.Size(), after.Size(),
		)
	}
}
