package main

import (
	"bitcask-go/internal/storage"
	"bytes"
	"fmt"
	"log"
)

func main() {
	fmt.Println("Hello, Bitcask!")
	db, err := storage.Open("data")
	if err != nil {
		fmt.Printf("Failed to open Bitcask: %v\n", err)
		return
	}
	defer db.Close()

	value := bytes.Repeat([]byte("x"), 1024*1024) // 1 MiB

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := db.Put(key, value); err != nil {
			log.Fatalf("Put failed: %v", err)
		}
	}

	db.PrintIndex()
	//Overwrite a key so an older record becomes stale.
	if err := db.Put("key-0", []byte("latest-value")); err != nil {
		log.Fatalf("overwrite failed: %v", err)
	}
	// Now compact.
	if err := db.Compaction(); err != nil {
		log.Fatalf("Compaction failed: %v", err)
	}

	// Verify the latest value.
	got, err := db.Get("key-0")
	if err != nil {
		log.Fatalf("Get failed: %v", err)
	}
	if string(got) != "latest-value" {
		log.Fatalf("got %q, want latest-value", got)
	}
	db.PrintIndex()
	// Now compact.
	if err := db.Compaction(); err != nil {
		log.Fatalf("Compaction failed: %v", err)
	}
	db.PrintIndex()
	// Verify all values after compaction.
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)

		got, err := db.Get(key)
		if err != nil {
			log.Fatalf("Get(%s) failed: %v", key, err)
		}

		if i == 0 {
			if string(got) != "latest-value" {
				log.Fatalf("unexpected value for %s: %q", key, got)
			}
		} else if len(got) != 1024*1024 {
			log.Fatalf("unexpected value size for %s: got %d bytes", key, len(got))
		}

		fmt.Printf("%s verified: %d bytes\n", key, len(got))
	}
	// keys := []string{"key-0", "key-1", "key-2", "key-3", "name2"}

	// for _, key := range keys {
	// 	value, err := db.Get(key)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}

	// 	fmt.Printf("%s = %s\n", key, value)

	//}
}
