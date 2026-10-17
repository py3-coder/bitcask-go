package storage

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentPutAndGet(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const (
		writers         = 5
		readers         = 5
		writesPerWorker = 100
	)

	var wg sync.WaitGroup

	// Writers use separate keys so final values are predictable.
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

	// Readers repeatedly access keys while writers are active.
	for r := 0; r < readers; r++ {
		wg.Add(1)

		go func(readerID int) {
			defer wg.Done()

			for i := 0; i < writesPerWorker; i++ {
				workerID := i % writers
				key := fmt.Sprintf("worker-%d-key-%d", workerID, i)

				value, err := db.Get(key)
				if err != nil {
					// A key might not have been written yet.
					// Missing keys are valid during concurrent execution.
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

	// Verify every written key after all goroutines have finished.
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

	const workers = 100

	var wg sync.WaitGroup
	errors := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			value := []byte(fmt.Sprintf("value-%d", id))
			if err := db.Put("shared-key", value); err != nil {
				errors <- fmt.Errorf("worker %d: %w", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	// Capture the value after every writer has finished.
	finalValue, err := db.Get("shared-key")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen to verify the index is rebuilt consistently.
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
