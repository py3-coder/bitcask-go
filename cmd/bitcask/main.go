package main

import (
	"bitcask-go/internal/storage"
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

	// if err := db.Put("name", []byte("Saurabh")); err != nil {
	// 	fmt.Printf("Failed to put record: %v\n", err)
	// }
	// if err := db.Put("name2", []byte("Shivangi")); err != nil {
	// 	fmt.Printf("Failed to put record: %v\n", err)
	// }
	// // Insert new records
	// if err := db.Put("language", []byte("Go")); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := db.Put("framework", []byte("Spring Boot")); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := db.Put("database", []byte("PostgreSQL")); err != nil {
	// 	log.Fatal(err)
	// }

	// // Overwrite an existing key
	// if err := db.Put("language", []byte("Java")); err != nil {
	// 	log.Fatal(err)
	// }

	db.PrintIndex()

	// value, err := db.Get("name")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println("name =", string(value))

	keys := []string{"language", "framework", "database" ,"name", "name2"}

	for _, key := range keys {
		value, err := db.Get(key)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s = %s\n", key, value)
	}
}
