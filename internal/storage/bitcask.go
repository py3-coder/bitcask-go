package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Entry struct {
	fileId    int
	offset    int64
	size      int64
	isDeleted bool
}

const maxRecordSize int64 = 2 << 20       // 2MB
const maxFileSize int64 = 2 * 1024 * 1024 // 2MB
const tombstone = "__BITCASK_TOMBSTONE__"

type Bitcask struct {
	mu           sync.RWMutex
	dir          string
	activeFileId int
	activeFile   *os.File
	writeOffset  int64
	index        map[string]Entry
}

func Open(dir string) (*Bitcask, error) {
	//create or open the dir.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}
	//open the data file:
	path := dir + "/000001.data"
	//open mode are read-write, create if not exists, append to the end of the file.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
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
		mu:           sync.RWMutex{},
		dir:          dir,
		activeFileId: 1,
		activeFile:   file,
		writeOffset:  info.Size(),
		index:        make(map[string]Entry),
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
	files, err := db.GetDataFiles()
	if err != nil {
		return fmt.Errorf("failed to get data files: %v", err)
	}
	for i, file := range files {
		isActiveFile := i == len(files)-1
		filePath := filepath.Join(db.dir, file)
		//open the file for reading:
		fileID, err := strconv.Atoi(strings.TrimSuffix(file, ".data"))
		if err != nil {
			return fmt.Errorf("invalid data filename %q: %w", file, err)
		}

		currfile, err := os.OpenFile(filePath, os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
		//read the file and rebuild the index:
		//move the cursor to the start of the file:
		_, err = currfile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to start of file: %v", err)
		}
		info, err := currfile.Stat()
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		fileSize := info.Size()
		var offset int64 = 0
		for offset < fileSize {
			//reset the index and handle short headers
			remaining := fileSize - offset
			if remaining < 8 {
				if err := handleIncompleteRecord(currfile, offset, isActiveFile); err != nil {
					return err
				}
				fileSize = offset
				break
			}
			//read the key length and value length in buffer header:
			header := make([]byte, 8)
			if _, err := io.ReadFull(currfile, header); err != nil {
				return fmt.Errorf("failed to read record header: %v", err)
			}
			keyLen := int64(binary.BigEndian.Uint32(header[0:4]))
			valueLen := int64(binary.BigEndian.Uint32(header[4:8]))
			recordSize := 8 + keyLen + valueLen

			if recordSize > maxRecordSize {
				return fmt.Errorf(
					"invalid record size %d at offset %d: exceeds maximum allowed size",
					recordSize,
					offset,
				)
			}

			//handle a record whose body is incomplete
			if recordSize > remaining {
				if err := currfile.Truncate(offset); err != nil {
					return fmt.Errorf("truncate incomplete record: %w", err)
				}

				fileSize = offset
				break
			}
			//buffer to hold the key and value data:
			record := make([]byte, recordSize)
			//set the header in the record buffer:
			copy(record[0:8], header)
			//read the key and value data:
			if _, err := io.ReadFull(currfile, record[8:]); err != nil {
				return fmt.Errorf("failed to read record: %v", err)
			}
			//decode the record to get the key:
			key, value, err := decodeRecord(record)
			if err != nil {
				return fmt.Errorf("failed to decode record: %v", err)
			}
			//update the index: but also need extra check for deleted value:
			if bytes.Equal(value, []byte(tombstone)) {
				db.index[key] = Entry{
					fileId:    fileID,
					offset:    offset,
					size:      recordSize,
					isDeleted: true,
				}
			} else {
				db.index[key] = Entry{
					fileId:    fileID,
					offset:    offset,
					size:      recordSize,
					isDeleted: false,
				}
			}
			offset += recordSize
		}
		if isActiveFile {
			db.writeOffset = offset
		}
		currfile.Close()
	}
	return nil
}

func (db *Bitcask) Close() error {
	//thread safe:
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.activeFile.Close()
}

//the reason we took out is actually while delete call the put correct
// then put would have take own lock and after done it have release the lock after reasing suppose some
//put/delete change the value
// to make delete atomic::

func (db *Bitcask) Put(key string, value []byte) error {
	//lock the mutex to ensure thread safety during write operations
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.putLocked(key, value)
}

// Put stores a key-value pair in the Bitcask database.
func (db *Bitcask) putLocked(key string, value []byte) error {
	//encode the record:
	record, err := encodeRecord(key, value)
	if err != nil {
		return fmt.Errorf("failed to encode record: %v", err)
	}

	//if user want to write more the maxFileSize, we will return error:
	//default size we picked is 2MB
	if int64(len(record)) > maxFileSize {
		return fmt.Errorf("record size %d exceeds maximum file size %d", len(record), maxFileSize)
	}

	//check the file size limit:
	if db.writeOffset+int64(len(record)) > maxFileSize {
		//we are not able write in currfile:
		//creat new file and switch the active file to the new file:
		if err := db.CreateNewDataFile(); err != nil {
			return fmt.Errorf("failed to create new data file: %v", err)
		}
	}

	//get the curr write offset:
	//capture this AFTER rotation, because the new file starts at offset 0.
	offset := db.writeOffset

	//seek to the write offset in the file:
	if _, err := db.activeFile.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek to write offset: %w", err)
	}
	//write the record to the file:
	n, err := db.activeFile.Write(record)
	if err != nil || n != len(record) {
		//Best effort to rollback the write offset to previous boundary:
		rollbackErr := db.activeFile.Truncate(offset)
		if rollbackErr != nil {
			return fmt.Errorf("failed to write record: %v, rollback failed: %v", err, rollbackErr)
		}
		/*
			If rollback itself fails, the file may be left in an uncertain state.
			The safest policy is to mark the database as unhealthy and reject subsequent writes
			until it is reopened and recovery runs.
		*/

		if _, seekErr := db.activeFile.Seek(offset, io.SeekStart); seekErr != nil {
			return fmt.Errorf(
				"write failed (wrote %d/%d bytes, err=%v); rollback succeeded but seek failed: %w",
				n, len(record), err, seekErr,
			)
		}
		if err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}

		return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(record))
	}

	//sync : os cache the page in ram , doing syc that data is written to disk:
	//one thing we are loosing if we do not sync is if the system crashes
	//before the data is flushed to disk, we will lose the data.
	if err := db.activeFile.Sync(); err != nil {
		return fmt.Errorf("sync data file: %w", err)
	}

	//increase the write offset:
	db.writeOffset += int64(n)
	//update the index:
	db.index[key] = Entry{
		fileId:    db.activeFileId,
		offset:    offset,
		size:      int64(n),
		isDeleted: false,
	}
	return nil
}

func (db *Bitcask) Get(key string) ([]byte, error) {
	//lock the mutex to ensure thread safety during read operations
	db.mu.RLock()
	defer db.mu.RUnlock()

	//find the key in the index:
	entry, ok := db.index[key]
	if !ok || entry.isDeleted {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	// //move the cursor to the file offset:
	// _, err := db.file.Seek(entry.offset, io.SeekStart)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to seek to offset: %v", err)
	// }
	// //make a buffer to read the record:
	record := make([]byte, entry.size)
	// //read the record from the file:
	// if _, err := io.ReadFull(db.file, record); err != nil {
	// 	return nil, fmt.Errorf("failed to read record: %v", err)
	// }
	//Thread Safety : Issue:
	//Issue with Seek and ReadFull we need two system calls, one for seek and one for read,
	// which is not atomic. If the file is changed between these two calls,
	// we may read the wrong data. So we will use ReadAt which is atomic and reads from a specific offset.

	// read the record from entry;s fileId and offset not from active file:
	path := db.dir
	// open the file with the given fileId:
	fileName := fmt.Sprintf("%06d.data", entry.fileId)
	filePath := filepath.Join(path, fileName)
	//open the file for reading:
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file: %w", err)
	}
	defer file.Close()
	//read record from the fileId where the key is stored at given offset:
	n, err := file.ReadAt(record, entry.offset)
	if err != nil {
		return nil, fmt.Errorf("failed to read record: %w", err)
	}
	if n != len(record) {
		return nil, fmt.Errorf(
			"incomplete record read: got %d bytes, expected %d",
			n, len(record),
		)
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
		fmt.Printf("  %s: offset=%d, size=%d , fileId=%d\n", key, entry.offset, entry.size, entry.fileId)
	}
}

// create a new data file and switch the active file to the new file and close the prev file:
func (db *Bitcask) CreateNewDataFile() error {
	//increment the file id and create a new file name:
	newFileID := db.activeFileId + 1
	newFileName := fmt.Sprintf("%06d.data", newFileID)
	newFilePath := filepath.Join(db.dir, newFileName)

	// open the new file before switching away from the old one:
	newFile, err := os.OpenFile(
		newFilePath,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		0644,
	)
	if err != nil {
		return fmt.Errorf("failed to create new data file: %w", err)
	}

	oldFile := db.activeFile
	// switch active file state:
	db.activeFile = newFile
	db.activeFileId = newFileID
	db.writeOffset = 0

	// close the prev file:
	// close the prev file:
	if oldFile != nil {
		if err := oldFile.Close(); err != nil {
			// The new file is already active, so don't pretend
			// the switch was rolled back.
			return fmt.Errorf("switched to new segment %d, but failed to close old file: %w", newFileID, err)
		}
	}

	return nil
}

// Get Data files:
func (db *Bitcask) GetDataFiles() ([]string, error) {
	files, err := os.ReadDir(db.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	var dataFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".data" {
			dataFiles = append(dataFiles, file.Name())
		}
	}
	return dataFiles, nil
}

func handleIncompleteRecord(file *os.File, offset int64, isActiveFile bool) error {
	if !isActiveFile {
		return fmt.Errorf(
			"incomplete record in immutable segment %s at offset %d",
			file.Name(),
			offset,
		)
	}

	if err := file.Truncate(offset); err != nil {
		return fmt.Errorf("failed to truncate incomplete record: %w", err)
	}

	return nil
}
func (db *Bitcask) Compaction() error {
	//lock the mutex to ensure thread safety during compaction
	db.mu.Lock()
	defer db.mu.Unlock()

	//get the list of data files:
	files, err := db.GetDataFiles()
	if err != nil {
		return fmt.Errorf("failed to get data files: %w", err)
	}

	var immutableFiles []string
	activeFileName := fmt.Sprintf("%06d.data", db.activeFileId)

	for _, file := range files {
		if file == activeFileName {
			continue
		}
		immutableFiles = append(immutableFiles, file)
	}

	// nothing to compact:
	if len(immutableFiles) == 0 {
		return nil
	}

	//make a new file for compaction:
	newFileID := db.activeFileId + 1
	newFile, err := createCompactionFile(db.dir, newFileID)
	if err != nil {
		return fmt.Errorf("failed to create new data file for compaction: %w", err)
	}

	var newOffset int64 = 0
	compactedEntries := make(map[string]Entry)
	compactionFiles := []*os.File{newFile}

	// close all temporary output files if we return early:
	defer func() {
		for _, f := range compactionFiles {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	for _, file := range immutableFiles {
		filePath := filepath.Join(db.dir, file)

		//open the file for reading:
		fileID, err := strconv.Atoi(strings.TrimSuffix(file, ".data"))
		if err != nil {
			return fmt.Errorf("invalid data filename %q: %w", file, err)
		}

		currfile, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}

		//read the file and validate against the index:
		info, err := currfile.Stat()
		if err != nil {
			_ = currfile.Close()
			return fmt.Errorf("failed to get file info: %w", err)
		}

		fileSize := info.Size()
		var offset int64 = 0

		for offset < fileSize {
			//reset the index and handle short headers
			remaining := fileSize - offset
			if remaining < 8 {
				_ = currfile.Close()
				return fmt.Errorf("incomplete record in immutable segment %s at offset %d", filePath, offset)
			}

			//read the key length and value length in buffer header:
			header := make([]byte, 8)
			if _, err := currfile.ReadAt(header, offset); err != nil {
				_ = currfile.Close()
				return fmt.Errorf("failed to read record header: %w", err)
			}

			keyLen := int64(binary.BigEndian.Uint32(header[0:4]))
			valueLen := int64(binary.BigEndian.Uint32(header[4:8]))
			recordSize := int64(8) + keyLen + valueLen

			if recordSize < 8 || recordSize > maxRecordSize {
				_ = currfile.Close()
				return fmt.Errorf(
					"invalid record size %d at offset %d: exceeds maximum allowed size",
					recordSize, offset,
				)
			}

			//handle a record whose body is incomplete
			if recordSize > remaining {
				_ = currfile.Close()
				return fmt.Errorf(
					"incomplete record body in immutable segment %s at offset %d",
					filePath, offset,
				)
			}

			//buffer to hold the key and value data:
			record := make([]byte, int(recordSize))

			//read the key and value data:
			if _, err := currfile.ReadAt(record, offset); err != nil {
				_ = currfile.Close()
				return fmt.Errorf("failed to read record: %w", err)
			}

			//decode the record to get the key:
			key, _, err := decodeRecord(record)
			if err != nil {
				_ = currfile.Close()
				return fmt.Errorf("failed to decode record: %w", err)
			}

			// match the record with the index.
			entry, exists := db.index[key]
			if exists && entry.fileId == fileID && entry.offset == offset {
				// this is the latest record keep it:
				//check the file size limit:
				if newOffset+recordSize > maxFileSize {
					//we are not able write in currfile:
					//creat new file and switch the active file to the new file:
					//need to create new file again:
					if recordSize > maxFileSize {
						_ = currfile.Close()
						return fmt.Errorf("record exceeds maximum file size")
					}
					//sync : os cache the page in ram , doing syc that data is written to disk:
					//one thing we are loosing if we do not sync is if the system crashes
					//before the data is flushed to disk, we will lose the data.
					// Sync each output file after copying is complete, not each record.

					if err := newFile.Sync(); err != nil {
						_ = currfile.Close()
						return fmt.Errorf("sync compacted data file: %w", err)
					}

					newFileID++
					//new next file create
					nextFile, err := createCompactionFile(db.dir, newFileID)
					if err != nil {
						_ = currfile.Close()
						return fmt.Errorf("failed to create next compaction file: %w", err)
					}
					//updated the newfile with next file
					newFile = nextFile
					//add the newFile in list:
					compactionFiles = append(compactionFiles, newFile)
					newOffset = 0
				}

				//seek to the write offset in the file:
				// WriteAt uses the explicit offset, so no Seek is needed.
				//write the record to the file:
				n, err := newFile.WriteAt(record, newOffset)
				if err != nil || n != len(record) {
					_ = currfile.Close()
					if err != nil {
						return fmt.Errorf("failed to write record: %w", err)
					}
					return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(record))
				}
				//update the index:
				compactedEntries[key] = Entry{
					fileId: newFileID,
					offset: newOffset,
					size:   recordSize,
				}

				//increase the write offset:
				newOffset += recordSize
			} else {
				//stale record skip it:
			}

			// advance the source offset:
			offset += recordSize
		}

		if err := currfile.Close(); err != nil {
			return fmt.Errorf("failed to close source file %s: %w", filePath, err)
		}
	}

	// sync all compacted output files before changing the index:
	for _, f := range compactionFiles {
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync compacted data file: %w", err)
		}
	}

	// close compacted output files before installing the new active file:
	for i, f := range compactionFiles {
		if err := f.Close(); err != nil {
			compactionFiles[i] = nil
			return fmt.Errorf("close compacted data file: %w", err)
		}
		compactionFiles[i] = nil
	}

	// create a new active file after the compacted seg:
	nextActiveID := newFileID + 1
	nextActiveName := fmt.Sprintf("%06d.data", nextActiveID)
	nextActivePath := filepath.Join(db.dir, nextActiveName)

	nextActiveFile, err := os.OpenFile(nextActivePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("failed to create new active file: %w", err)
	}

	// update the index:
	for key, entry := range compactedEntries {
		db.index[key] = entry
	}

	// switch the active file state:
	oldActiveFile := db.activeFile
	db.activeFile = nextActiveFile
	db.activeFileId = nextActiveID
	db.writeOffset = 0

	if oldActiveFile != nil {
		if err := oldActiveFile.Close(); err != nil {
			return fmt.Errorf("failed to close previous active file: %w", err)
		}
	}

	// delete the old immutable files only after the compacted output sync:
	for _, file := range immutableFiles {
		filePath := filepath.Join(db.dir, file)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove old segment %s: %w", file, err)
		}
	}

	return nil
}

func createCompactionFile(dir string, fileID int) (*os.File, error) {
	fileName := fmt.Sprintf("%06d.data", fileID)
	filePath := filepath.Join(dir, fileName)

	return os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
}

// delete - kind of put internally ::::
func (db *Bitcask) Delete(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.putLocked(key, []byte(tombstone)); err != nil {
		return err
	}

	entry, exists := db.index[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	entry.isDeleted = true
	db.index[key] = entry

	return nil
}
