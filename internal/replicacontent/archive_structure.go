package replicacontent

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf8"
)

const (
	zipEndRecordSize       = 22
	zipCentralHeaderSize   = 46
	zipLocalHeaderSize     = 30
	zipEndSignature        = 0x06054b50
	zipCentralSignature    = 0x02014b50
	zipLocalSignature      = 0x04034b50
	zipDescriptorSignature = 0x08074b50
	zip64LocatorSignature  = 0x07064b50
)

type zipDirectory struct {
	offset     int64
	size       int64
	entryCount int
	endOffset  int64
}

type zipCentralEntry struct {
	rawName          []byte
	name             string
	creatorVersion   uint16
	readerVersion    uint16
	flags            uint16
	method           uint16
	checksum         uint32
	compressedSize   uint32
	uncompressedSize uint32
	externalAttrs    uint32
	localOffset      uint32
}

type zipLocalRegion struct {
	start int64
	end   int64
}

func validateArchiveStructure(file *os.File, size int64) ([]zipCentralEntry, error) {
	directory, err := readZIPDirectory(file, size)
	if err != nil {
		return nil, err
	}
	entries, err := readZIPCentralEntries(file, directory)
	if err != nil {
		return nil, err
	}
	if err := validateZIPLocalEntries(file, directory, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func readZIPDirectory(file *os.File, size int64) (zipDirectory, error) {
	const maxCommentSize = 1<<16 - 1
	if size < zipEndRecordSize {
		return zipDirectory{}, errors.New("Website Replica ZIP end record is missing")
	}
	tailSize := int64(zipEndRecordSize + maxCommentSize + 20)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if err := readZIPAt(file, tail, size-tailSize); err != nil {
		return zipDirectory{}, fmt.Errorf("read Website Replica ZIP end record: %w", err)
	}
	for offset := len(tail) - zipEndRecordSize; offset >= 0; offset-- {
		if binary.LittleEndian.Uint32(tail[offset:]) != zipEndSignature {
			continue
		}
		commentSize := int(binary.LittleEndian.Uint16(tail[offset+20:]))
		if offset+zipEndRecordSize+commentSize != len(tail) {
			continue
		}
		endOffset := size - tailSize + int64(offset)
		if endOffset >= 20 {
			locator := make([]byte, 4)
			if err := readZIPAt(file, locator, endOffset-20); err != nil {
				return zipDirectory{}, err
			}
			if binary.LittleEndian.Uint32(locator) == zip64LocatorSignature {
				return zipDirectory{}, errors.New("Website Replica ZIP64 metadata is not supported")
			}
		}
		disk := binary.LittleEndian.Uint16(tail[offset+4:])
		centralDisk := binary.LittleEndian.Uint16(tail[offset+6:])
		diskEntries := binary.LittleEndian.Uint16(tail[offset+8:])
		totalEntries := binary.LittleEndian.Uint16(tail[offset+10:])
		centralSize := binary.LittleEndian.Uint32(tail[offset+12:])
		centralOffset := binary.LittleEndian.Uint32(tail[offset+16:])
		if disk != 0 || centralDisk != 0 || diskEntries != totalEntries {
			return zipDirectory{}, errors.New("Website Replica ZIP uses unsupported multi-disk metadata")
		}
		if totalEntries == 0xffff || centralSize == 0xffffffff || centralOffset == 0xffffffff {
			return zipDirectory{}, errors.New("Website Replica ZIP64 metadata is not supported")
		}
		if int(totalEntries) > MaxArchiveEntries {
			return zipDirectory{}, errors.New("Website Replica ZIP contains too many entries")
		}
		offset64 := int64(centralOffset)
		size64 := int64(centralSize)
		if offset64 > endOffset || size64 > endOffset-offset64 || offset64+size64 != endOffset {
			return zipDirectory{}, errors.New("Website Replica ZIP central directory bounds are invalid")
		}
		return zipDirectory{offset: offset64, size: size64, entryCount: int(totalEntries), endOffset: endOffset}, nil
	}
	return zipDirectory{}, errors.New("Website Replica ZIP end record is invalid")
}

func readZIPCentralEntries(file *os.File, directory zipDirectory) ([]zipCentralEntry, error) {
	entries := make([]zipCentralEntry, 0, directory.entryCount)
	cursor := directory.offset
	end := directory.offset + directory.size
	for index := 0; index < directory.entryCount; index++ {
		if cursor > end-zipCentralHeaderSize {
			return nil, errors.New("Website Replica ZIP central directory ended early")
		}
		header := make([]byte, zipCentralHeaderSize)
		if err := readZIPAt(file, header, cursor); err != nil {
			return nil, err
		}
		if binary.LittleEndian.Uint32(header) != zipCentralSignature {
			return nil, errors.New("Website Replica ZIP central entry signature is invalid")
		}
		nameLength := int64(binary.LittleEndian.Uint16(header[28:]))
		extraLength := int64(binary.LittleEndian.Uint16(header[30:]))
		commentLength := int64(binary.LittleEndian.Uint16(header[32:]))
		variableLength := nameLength + extraLength + commentLength
		if nameLength == 0 || nameLength > MaxArchivePathBytes || variableLength > end-cursor-zipCentralHeaderSize {
			return nil, errors.New("Website Replica ZIP central entry bounds are invalid")
		}
		if binary.LittleEndian.Uint16(header[34:]) != 0 {
			return nil, errors.New("Website Replica ZIP central entry uses another disk")
		}
		compressedSize := binary.LittleEndian.Uint32(header[20:])
		uncompressedSize := binary.LittleEndian.Uint32(header[24:])
		localOffset := binary.LittleEndian.Uint32(header[42:])
		if compressedSize == 0xffffffff || uncompressedSize == 0xffffffff || localOffset == 0xffffffff {
			return nil, errors.New("Website Replica ZIP64 entry metadata is not supported")
		}
		variable := make([]byte, nameLength+extraLength)
		if err := readZIPAt(file, variable, cursor+zipCentralHeaderSize); err != nil {
			return nil, err
		}
		rawName := append([]byte(nil), variable[:nameLength]...)
		flags := binary.LittleEndian.Uint16(header[8:])
		name, err := decodeZIPName(rawName, flags)
		if err != nil {
			return nil, err
		}
		if err := validateZIPExtra(variable[nameLength:]); err != nil {
			return nil, err
		}
		entries = append(entries, zipCentralEntry{
			rawName:          rawName,
			name:             name,
			creatorVersion:   binary.LittleEndian.Uint16(header[4:]),
			readerVersion:    binary.LittleEndian.Uint16(header[6:]),
			flags:            flags,
			method:           binary.LittleEndian.Uint16(header[10:]),
			checksum:         binary.LittleEndian.Uint32(header[16:]),
			compressedSize:   compressedSize,
			uncompressedSize: uncompressedSize,
			externalAttrs:    binary.LittleEndian.Uint32(header[38:]),
			localOffset:      localOffset,
		})
		cursor += zipCentralHeaderSize + variableLength
	}
	if cursor != end {
		return nil, errors.New("Website Replica ZIP central directory size or entry count is inconsistent")
	}
	return entries, nil
}

func validateZIPLocalEntries(file *os.File, directory zipDirectory, entries []zipCentralEntry) error {
	regions := make([]zipLocalRegion, 0, len(entries))
	offsets := make(map[uint32]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := offsets[entry.localOffset]; exists {
			return errors.New("Website Replica ZIP reuses a local entry offset")
		}
		offsets[entry.localOffset] = struct{}{}
		start := int64(entry.localOffset)
		if start > directory.offset-zipLocalHeaderSize {
			return errors.New("Website Replica ZIP local entry is outside the file-data area")
		}
		header := make([]byte, zipLocalHeaderSize)
		if err := readZIPAt(file, header, start); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header) != zipLocalSignature ||
			binary.LittleEndian.Uint16(header[4:]) != entry.readerVersion ||
			binary.LittleEndian.Uint16(header[6:]) != entry.flags ||
			binary.LittleEndian.Uint16(header[8:]) != entry.method {
			return errors.New("Website Replica ZIP local and central headers disagree")
		}
		nameLength := int64(binary.LittleEndian.Uint16(header[26:]))
		extraLength := int64(binary.LittleEndian.Uint16(header[28:]))
		dataStart := start + zipLocalHeaderSize + nameLength + extraLength
		if nameLength != int64(len(entry.rawName)) || dataStart > directory.offset || int64(entry.compressedSize) > directory.offset-dataStart {
			return errors.New("Website Replica ZIP local entry bounds are invalid")
		}
		variable := make([]byte, nameLength+extraLength)
		if err := readZIPAt(file, variable, start+zipLocalHeaderSize); err != nil {
			return err
		}
		if !bytes.Equal(variable[:nameLength], entry.rawName) {
			return errors.New("Website Replica ZIP local and central names disagree")
		}
		if err := validateZIPExtra(variable[nameLength:]); err != nil {
			return err
		}
		if entry.flags&0x8 == 0 && (binary.LittleEndian.Uint32(header[14:]) != entry.checksum ||
			binary.LittleEndian.Uint32(header[18:]) != entry.compressedSize ||
			binary.LittleEndian.Uint32(header[22:]) != entry.uncompressedSize) {
			return errors.New("Website Replica ZIP local entry sizes or checksum disagree")
		}
		regionEnd := dataStart + int64(entry.compressedSize)
		if entry.flags&0x8 != 0 {
			var err error
			regionEnd, err = validateZIPDescriptor(file, regionEnd, directory.offset, entry)
			if err != nil {
				return err
			}
		}
		regions = append(regions, zipLocalRegion{start: start, end: regionEnd})
	}
	sort.Slice(regions, func(left, right int) bool { return regions[left].start < regions[right].start })
	for index := 1; index < len(regions); index++ {
		if regions[index-1].end > regions[index].start {
			return errors.New("Website Replica ZIP local entries overlap")
		}
	}
	return nil
}

func validateZIPDescriptor(file *os.File, offset, centralOffset int64, entry zipCentralEntry) (int64, error) {
	first := make([]byte, 4)
	if err := readZIPAt(file, first, offset); err != nil {
		return 0, err
	}
	hasSignature := binary.LittleEndian.Uint32(first) == zipDescriptorSignature
	remainderLength := 8
	if hasSignature {
		remainderLength = 12
	}
	if offset+4+int64(remainderLength) > centralOffset {
		return 0, errors.New("Website Replica ZIP data descriptor overlaps the central directory")
	}
	remainder := make([]byte, remainderLength)
	if err := readZIPAt(file, remainder, offset+4); err != nil {
		return 0, err
	}
	checksum := binary.LittleEndian.Uint32(first)
	compressedOffset := 0
	if hasSignature {
		checksum = binary.LittleEndian.Uint32(remainder)
		compressedOffset = 4
	}
	if checksum != entry.checksum || binary.LittleEndian.Uint32(remainder[compressedOffset:]) != entry.compressedSize ||
		binary.LittleEndian.Uint32(remainder[compressedOffset+4:]) != entry.uncompressedSize {
		return 0, errors.New("Website Replica ZIP data descriptor disagrees with the central entry")
	}
	return offset + 4 + int64(remainderLength), nil
}

func validateZIPReader(reader *zip.Reader, entries []zipCentralEntry) error {
	if len(reader.File) != len(entries) || len(reader.File) > MaxArchiveEntries {
		return errors.New("Website Replica ZIP reader entry count disagrees with the central directory")
	}
	for index, file := range reader.File {
		entry := entries[index]
		if file.Name != entry.name || file.CreatorVersion != entry.creatorVersion || file.ReaderVersion != entry.readerVersion ||
			file.Flags != entry.flags || file.Method != entry.method || file.CRC32 != entry.checksum ||
			file.CompressedSize64 != uint64(entry.compressedSize) || file.UncompressedSize64 != uint64(entry.uncompressedSize) ||
			file.ExternalAttrs != entry.externalAttrs {
			return errors.New("Website Replica ZIP reader interpretation disagrees with its raw central directory")
		}
	}
	return nil
}

func decodeZIPName(raw []byte, flags uint16) (string, error) {
	if flags&0x800 == 0 {
		for _, value := range raw {
			if value >= 0x80 {
				return "", errors.New("Website Replica ZIP contains a non-portable legacy-encoded name")
			}
		}
		return string(raw), nil
	}
	if !utf8.Valid(raw) {
		return "", errors.New("Website Replica ZIP contains an invalid UTF-8 name")
	}
	return string(raw), nil
}

func validateZIPExtra(extra []byte) error {
	for len(extra) > 0 {
		if len(extra) < 4 {
			return errors.New("Website Replica ZIP contains a malformed extra field")
		}
		fieldID := binary.LittleEndian.Uint16(extra)
		fieldSize := int(binary.LittleEndian.Uint16(extra[2:]))
		if fieldSize > len(extra)-4 {
			return errors.New("Website Replica ZIP contains a truncated extra field")
		}
		if fieldID == 0x0001 {
			return errors.New("Website Replica ZIP64 extra fields are not supported")
		}
		extra = extra[4+fieldSize:]
	}
	return nil
}

func readZIPAt(file *os.File, buffer []byte, offset int64) error {
	if offset < 0 {
		return errors.New("Website Replica ZIP contains a negative file offset")
	}
	_, err := io.ReadFull(io.NewSectionReader(file, offset, int64(len(buffer))), buffer)
	return err
}
