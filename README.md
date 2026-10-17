# Bitcask-Go

A Bitcask-inspired key-value storage engine built from scratch in Go to explore database internals and storage systems.

## 🚀 Features

- **Append-only storage** — records are appended to a data file.
- **In-memory hash index** — maps keys to record offsets and sizes for fast lookups.
- **Record encoding** — binary record format with key and value lengths.
- **Index recovery** — rebuilds the in-memory index by scanning the data file on startup.
- **Latest-write-wins** — retrieves the most recent value written for a key.

## 🛠️ Tech Stack

- **Language:** Go
- **Storage:** Append-only binary data file
- **Index:** In-memory Go map

## 📂 Project Structure

```text
bitcask/
├── cmd/
│   └── main.go
├── storage/
│   ├── bitcask.go
│   └── record.go
├── data/
│   └── 000001.data
├── go.mod
└── README.md
```

## ⚙️ Getting Started

### Prerequisites

- Go installed on your machine
- Git

### Run the project

```bash
git clone https://github.com/py3-coder/bitcask-go.git
cd bitcask-go
go run ./cmd
```

Update the repository URL if you choose a different GitHub repository name.

## 🧠 What I'm Learning

This project is a hands-on exploration of how key-value storage engines work internally:

- Binary record serialization and deserialization
- Append-only log-based storage
- In-memory indexing and offset-based reads
- Startup recovery and index reconstruction
- Storage-engine design trade-offs

## 🗺️ Roadmap

- [x] Append-only record storage
- [x] In-memory hash index
- [x] Startup index recovery
- [ ] Crash recovery and incomplete-write handling
- [ ] Delete operations and tombstones
- [ ] Concurrent reads and writes
- [ ] Disk durability and synchronization
- [ ] Log compaction and obsolete-record cleanup
- [ ] Benchmarks and performance testing

## ⚠️ Project Status

This is an experimental learning project, not a production-ready database. Durability guarantees, crash handling, concurrency safety, and compaction are still under development.

## 📚 Inspiration

Inspired by the Bitcask storage model and built to understand database internals by implementing the core concepts from scratch.

---

**Built with Go, one storage-engine concept at a time.**