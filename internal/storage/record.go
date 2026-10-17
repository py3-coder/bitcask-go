package storage

import (
	"encoding/binary"
	"fmt"
)

func encodeRecord(key string, value []byte) ([]byte, error) {
	if uint64(len(key)) > uint64(^uint32(0)) ||
		uint64(len(value)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("key or value too large")
	}
	//create a buffer to hold the encoded record
	buf := make([]byte, 8+len(key)+len(value))

	// encode the key length and key
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(key)))
	copy(buf[8:8+len(key)], key)

	// encode the value length and value
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(value)))
	copy(buf[8+len(key):8+len(key)+len(value)], value)

	return buf, nil
}

func decodeRecord(data []byte) (string, []byte, error) {
	if len(data) < 8 {
		return "", nil, fmt.Errorf("data too short to decode")
	}
	keyLen := binary.BigEndian.Uint32(data[0:4])
	valueLen := binary.BigEndian.Uint32(data[4:8])
	expectedSize := uint64(8) + uint64(keyLen) + uint64(valueLen)
	if expectedSize != uint64(len(data)) {
		return "", nil, fmt.Errorf("invalid data size")
	}
	keyStart := 8
	keyEnd := keyStart + int(keyLen)
	valueStart := keyEnd
	valueEnd := valueStart + int(valueLen)

	key := string(data[keyStart:keyEnd])
	value := data[valueStart:valueEnd]

	return key, value, nil
}
