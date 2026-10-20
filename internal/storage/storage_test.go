package storage

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentPutAndGet(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	const (
		writers         = 5
		readers         = 5
		writesPerWorker = 100
	)

	var wg sync.WaitGroup

	// Writers use distinct keys so their expected values are deterministic.
	for w := 0; w < writers; w++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < writesPerWorker; i++ {
				key := fmt.Sprintf("worker-%d-key-%d", workerID, i)
				value := []byte(fmt.Sprintf("value-%d-%d", workerID, i))

				if err := db.Put(key, value); err != nil {
					t.Errorf("Put(%q) failed: %v", key, err)
					return
				}
			}
		}(w)
	}

	// Reads may happen before a key has been written.
	for r := 0; r < readers; r++ {
		wg.Add(1)

		go func(readerID int) {
			defer wg.Done()

			for i := 0; i < writesPerWorker; i++ {
				workerID := i % writers
				key := fmt.Sprintf("worker-%d-key-%d", workerID, i)

				value, err := db.Get(key)
				if err != nil {
					continue
				}

				expected := fmt.Sprintf("value-%d-%d", workerID, i)
				if string(value) != expected {
					t.Errorf(
						"reader %d: key %q: expected %q, got %q",
						readerID, key, expected, value,
					)
					return
				}
			}
		}(r)
	}

	wg.Wait()

	// Once all writers finish, every key must exist with its expected value.
	for w := 0; w < writers; w++ {
		for i := 0; i < writesPerWorker; i++ {
			key := fmt.Sprintf("worker-%d-key-%d", w, i)
			expected := fmt.Sprintf("value-%d-%d", w, i)

			value, err := db.Get(key)
			if err != nil {
				t.Fatalf("Get(%q) failed: %v", key, err)
			}
			if string(value) != expected {
				t.Fatalf("key %q: expected %q, got %q",
					key, expected, value)
			}
		}
	}
}

func TestConcurrentWritesSameKey(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the currently assigned DB is closed even if a later
	// assertion fails. The variable is reassigned after reopening.
	defer func() {
		if db != nil {
			if err := db.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		}
	}()

	const workers = 100

	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			value := []byte(fmt.Sprintf("value-%d", id))
			if err := db.Put("shared-key", value); err != nil {
				errs <- fmt.Errorf("worker %d: %w", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// Don't continue if any write failed.
	if t.Failed() {
		return
	}

	finalValue, err := db.Get("shared-key")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = nil

	reopenedDB, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	db = reopenedDB

	recoveredValue, err := db.Get("shared-key")
	if err != nil {
		t.Fatal(err)
	}

	if string(recoveredValue) != string(finalValue) {
		t.Fatalf(
			"value changed after recovery: before=%q after=%q",
			finalValue, recoveredValue,
		)
	}
}

func TestDeleteWritesTombstone(t *testing.T) {
    db, err := Open(t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := db.Put("name", []byte("Saurabh")); err != nil {
        t.Fatal(err)
    }

    if err := db.Delete("name"); err != nil {
        t.Fatal(err)
    }

    value, err := db.Get("name")
    if err == nil {
        t.Fatalf("expected key-not-found error, got value %q", value)
    }
}

func TestDeleteKey(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put("name", []byte("Saurabh")); err != nil {
		t.Fatal(err)
	}

	if err := db.Delete("name"); err != nil {
		t.Fatal(err)
	}

	_, err = db.Get("name")
	if err == nil {
		t.Fatal("expected error when getting deleted key")
	}
}
func TestDeletePersistsAfterReopen(t *testing.T) {
    dir := t.TempDir()

    db, err := Open(dir)
    if err != nil {
        t.Fatal(err)
    }

    if err := db.Put("name", []byte("Saurabh")); err != nil {
        t.Fatal(err)
    }

    if err := db.Delete("name"); err != nil {
        t.Fatal(err)
    }

    if err := db.Close(); err != nil {
        t.Fatal(err)
    }

    // Reopen should rebuild the index from disk.
    db, err = Open(dir)
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    value, err := db.Get("name")
    if err == nil {
        t.Fatalf("expected deleted key to be missing, got %q", value)
    }
}