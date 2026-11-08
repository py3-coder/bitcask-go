# Bitcask-Go

A Bitcask-inspired key-value storage engine built from scratch in Go to explore database internals, append-only storage, indexing, recovery, and log compaction.

## 🚀 Features

- **Append-only storage** — Persists records in binary data files.
- **Segment rotation** — Rotates to a new data file when the configured segment size limit is reached (2 MiB by default).
- **Binary record encoding** — Stores keys and values with length-prefixed records.
- **In-memory hash index** — Maps keys to their latest record locations for efficient lookups.
- **Multi-segment index recovery** — Rebuilds the index by scanning data segments in order when the database opens.
- **Latest-write-wins semantics** — Ensures reads return the latest value written for a key.
- **Tombstone-based deletion** — Deletes keys by appending tombstone records instead of modifying existing data.
- **Crash recovery** — Handles incomplete trailing records by truncating recoverable tails in the active segment.
- **Corruption detection** — Rejects invalid records and corruption detected during index reconstruction.
- **Concurrent access** — Uses synchronization to coordinate concurrent reads and writes.
- **Log compaction** — Rewrites live records, removes obsolete versions, and cleans up deleted keys.
- **Compaction recovery** — Rebuilds the in-memory index after reopening a compacted database.

## 🛠️ Tech Stack

- **Language:** Go
- **Storage:** Append-only binary data files
- **Index:** In-memory hash map
- **Concurrency:** Go synchronization primitives
- **Testing:** Go testing package and race detector

## 📂 Project Structure

```text
bitcask-go/
├── cmd/
│   └── bitcask/
│       └── main.go
├── internal/
│   └── storage/
│       ├── bitcask.go
│       ├── record.go
│       ├── storage_test.go
│       ├── recovery_test.go
│       └── compaction_test.go
├── go.mod
├── .gitignore
└── README.md
```

*The structure above reflects the current project layout; adjust filenames if your repository differs.*

## ⚙️ Getting Started

### Prerequisites

- Go installed on your machine
- Git

### Clone the repository

```bash
git clone https://github.com/py3-coder/bitcask-go.git
cd bitcask-go
```

### Run the project

```bash
go run ./cmd/bitcask
```

### Run tests

```bash
go test ./...
```

Run the race detector to check for data races:

```bash
go test -race ./...
```

## 🧠 What I'm Learning

This project is a hands-on exploration of storage-engine internals, including:

- Binary record serialization and deserialization
- Append-only log-based storage
- Offset-based reads and in-memory indexing
- Segment rotation and multi-file management
- Startup recovery and index reconstruction
- Tombstone-based deletion
- Concurrent reads and writes
- Crash recovery and disk durability
- Log compaction and obsolete-record cleanup
- Storage-engine correctness and testing

## 🗺️ Roadmap

- [x] Append-only record storage
- [x] In-memory hash index
- [x] Multi-segment storage and rotation
- [x] Multi-segment index recovery
- [x] Incomplete trailing-record recovery
- [x] Corruption detection
- [x] Concurrent reads and writes
- [x] Tombstone-based deletion
- [x] Log compaction and obsolete-record cleanup
- [x] Compaction and deletion recovery tests
- [ ] Crash-safe compaction and stronger recovery guarantees
- [ ] Benchmarks and performance testing

## ⚠️ Project Status

This is an experimental learning project, not a production-ready database.

Although the project implements segment rotation, index recovery, concurrency controls, deletion, and compaction, further work is needed to validate crash consistency, compaction failure handling, durability guarantees, and behavior under heavy concurrent workloads.

## 🏗️ Architecture

```mermaid
flowchart TD
    Client["Client API"]

    subgraph Operations["Core Operations"]
        Put["Put(key, value)"]
        Get["Get(key)"]
        Delete["Delete(key)"]
        Compact["Compaction()"]
    end

    subgraph WritePath["Write Path"]
        Encode["Encode Binary Record"]
        Rotate{"Segment size limit reached?"}
        NewSegment["Create New Segment"]
        Append["Append Record"]
    end

    subgraph Storage["Persistent Storage"]
        Segments[("Numbered Data Segments")]
    end

    subgraph Indexing["In-Memory Index"]
        Index[("Hash Map: key → fileID, offset, size")]
        Deleted{"Entry deleted?"}
    end

    subgraph ReadPath["Read Path"]
        Locate["Locate Segment and Offset"]
        Read["ReadAt"]
        Decode["Decode Record"]
        Value["Return Value"]
        NotFound["Return Key Not Found"]
    end

    subgraph Recovery["Startup Recovery"]
        Open["Open Database"]
        Scan["Scan Data Segments"]
        Rebuild["Rebuild Index"]
        Tail["Recover Incomplete Active-Segment Tail"]
    end

    subgraph Compaction["Compaction"]
        Filter["Filter Stale Records"]
        Live["Copy Live Records"]
        Tombstones["Remove Deleted Keys"]
        Cleanup["Remove Old Segments"]
    end

    Client --> Put
    Client --> Get
    Client --> Delete
    Client --> Compact

    Put --> Encode
    Encode --> Rotate
    Rotate -- Yes --> NewSegment
    NewSegment --> Append
    Rotate -- No --> Append
    Append --> Segments
    Append --> Index

    Delete --> AppendTombstone["Append Tombstone"]
    AppendTombstone --> Segments
    AppendTombstone --> Index

    Get --> Index
    Index --> Deleted
    Deleted -- Yes --> NotFound
    Deleted -- No --> Locate
    Locate --> Read
    Read --> Segments
    Read --> Decode
    Decode --> Value

    Open --> Scan
    Scan --> Segments
    Scan --> Rebuild
    Scan --> Tail
    Tail --> Rebuild
    Rebuild --> Index

    Compact --> Filter
    Filter --> Live
    Filter --> Tombstones
    Live --> Cleanup
    Tombstones --> Cleanup
    Cleanup --> Segments
    Cleanup --> Index
```

The database appends records to numbered data segments and maintains an in-memory hash index pointing to the latest record for each key. Reads use the index to locate records on disk. Deletes append tombstones, while startup recovery reconstructs the index by scanning existing segments.

Compaction rewrites live records, discards stale versions, removes deleted keys, and cleans up obsolete segments. Crash-safe compaction and stronger recovery guarantees remain areas for further validation.

## 📚 Inspiration

Inspired by the [Bitcask storage model](https://riak.com/assets/bitcask-intro.pdf), with the goal of understanding database internals by implementing the core concepts from scratch.

---

**Built with Go, one storage-engine concept at a time.** 🚀