package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Entry struct {
	offset int64
	size   int64
}
type Bitcask struct {
	file        *os.File
	writeOffset int64
	index       map[string]Entry
}

func Open(dir string) (*Bitcask, error) {
	//creat or open the dir.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}
	//open the data file:
	path := dir + "/000001.data"
	//open mode are read-write, create if not exists, append to the end of the file.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	//get the info about file to get the current size:
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	//print the file path and size for debugging:
	fmt.Println("Data file:", path)
	fmt.Println("Existing file size:", info.Size())

	db := &Bitcask{
		file:        file,
		writeOffset: info.Size(),
		index:       make(map[string]Entry),
	}
	//rebuild the index from the existing data file:
	if err := db.RebuildIndex(); err != nil {
		file.Close()
		return nil, err
	}
	return db, nil
}

func (db *Bitcask) RebuildIndex() error {
	//clear the existing index: incase if any
	db.index = make(map[string]Entry)
	//move the cursor to the start of the file:
	_, err := db.file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to start of file: %v", err)
	}
	info, err := db.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}
	fileSize := info.Size()
	var offset int64 = 0
	for offset < fileSize {
		//read the key length and value length in buffer header:
		header := make([]byte, 8)
		if _, err := io.ReadFull(db.file, header); err != nil {
			return fmt.Errorf("failed to read record header: %v", err)
		}
		keyLen := int64(binary.BigEndian.Uint32(header[0:4]))
		valueLen := int64(binary.BigEndian.Uint32(header[4:8]))
		recordSize := 8 + keyLen + valueLen

		if recordSize > fileSize-offset {
			return fmt.Errorf(
				"incomplete record at offset %d: record size %d, remaining bytes %d",
				offset, recordSize, fileSize-offset,
			)
		}
		//buffer to hold the key and value data:
		record := make([]byte, recordSize)
		//set the header in the record buffer:
		copy(record[0:8], header)
		//read the key and value data:
		if _, err := io.ReadFull(db.file, record[8:]); err != nil {
			return fmt.Errorf("failed to read record: %v", err)
		}
		//decode the record to get the key:
		key, _, err := decodeRecord(record)
		if err != nil {
			return fmt.Errorf("failed to decode record: %v", err)
		}
		//update the index:
		db.index[key] = Entry{
			offset: offset,
			size:   recordSize,
		}
		offset += recordSize
	}
	db.writeOffset = offset
	return nil
}

func (db *Bitcask) Close() error {
	return db.file.Close()
}

func (db *Bitcask) Put(key string, value []byte) error {
	//encode the record:
	record, err := encodeRecord(key, value)
	if err != nil {
		return fmt.Errorf("failed to encode record: %v", err)
	}
	//get the curr write offset:
	offset := db.writeOffset
	//write the record to the file:
	n, err := db.file.Write(record)
	if err != nil {
		return fmt.Errorf("failed to write record: %v", err)
	}
	if n != len(record) {
		return fmt.Errorf("failed to write complete record")
	}
	//increase the write offset:
	db.writeOffset += int64(n)
	//update the index:
	db.index[key] = Entry{
		offset: offset,
		size:   int64(n),
	}
	db.DebugState()
	return nil
}

func (db *Bitcask) Get(key string) ([]byte, error) {
	//find the key in the index:
	entry, ok := db.index[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	//move the cursor to the file offset:
	_, err := db.file.Seek(entry.offset, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to offset: %v", err)
	}
	//make a buffer to read the record:
	record := make([]byte, entry.size)
	//read the record from the file:
	if _, err := io.ReadFull(db.file, record); err != nil {
		return nil, fmt.Errorf("failed to read record: %v", err)
	}
	//decode the record:
	_, value, err := decodeRecord(record)
	if err != nil {
		return nil, fmt.Errorf("failed to decode record: %v", err)
	}

	return value, nil

}
func (db *Bitcask) PrintIndex() {
	fmt.Println("Index:")
	for key, entry := range db.index {
		fmt.Printf("  %s: offset=%d, size=%d\n", key, entry.offset, entry.size)
	}
}
func (db *Bitcask) DebugState() {
	fmt.Println("Write offset:", db.writeOffset)
	fmt.Println("Index:", db.index)
}
