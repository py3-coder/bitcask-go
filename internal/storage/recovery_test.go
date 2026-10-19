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
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening should discard only the incomplete tail.
	db, err = Open(dir)
	if err != nil {
		t.Fatalf("Open after incomplete tail failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	got, err := db.Get("name")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Saurabh" {
		t.Fatalf("got %q, want %q", got, "Saurabh")
	}

	got, err = db.Get("language")
	if err != nil {
		t.Fatalf("valid record was lost: %v", err)
	}
	if string(got) != "Go" {
		t.Fatalf("got %q, want %q", got, "Go")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 37 {
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
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatalf("Open after incomplete body failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

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

	// The first record occupies 8 + 5 + 3 = 16 bytes.
	// Corrupt the second record's key-length field at offset 16.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}

	corruptHeader := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	if n, err := f.WriteAt(corruptHeader, 16); err != nil {
		f.Close()
		t.Fatal(err)
	} else if n != len(corruptHeader) {
		f.Close()
		t.Fatalf("wrote %d bytes of corrupt header, want %d", n, len(corruptHeader))
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	reopenedDB, err := Open(dir)
	if err == nil {
		if closeErr := reopenedDB.Close(); closeErr != nil {
			t.Errorf("Close unexpectedly opened DB: %v", closeErr)
		}
		t.Fatal("expected recovery error for corrupted record")
	}

	// Open must close any file handles it opened before returning an error.
	// This stat also checks that the file remains accessible.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("could not stat corrupted file after failed Open: %v", err)
	}

	if after.Size() != before.Size() {
		t.Fatalf(
			"recovery modified corrupted file: size before=%d after=%d",
			before.Size(), after.Size(),
		)
	}
}
