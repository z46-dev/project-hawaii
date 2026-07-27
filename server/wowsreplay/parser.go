package wowsreplay

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/blowfish"
)

const (
	defaultMaxMetadataSize     int64 = 4 << 20
	defaultMaxCompressedSize   int64 = 256 << 20
	defaultMaxDecompressedSize int64 = 1 << 30
	defaultMaxBlocks                 = 64
)

var (
	replayKey = [16]byte{
		0x29, 0xB7, 0xC9, 0x09, 0x38, 0x3F, 0x84, 0x88,
		0xFA, 0x98, 0xEC, 0x4E, 0x13, 0x19, 0x79, 0xFB,
	}

	ErrInvalidMagic     = errors.New("invalid WoWS replay magic")
	ErrInvalidContainer = errors.New("invalid WoWS replay container")
	ErrLimitExceeded    = errors.New("WoWS replay size limit exceeded")
	ErrTruncatedPacket  = errors.New("truncated WoWS replay packet")
)

type (
	// Parser controls replay safety limits and decoding behavior.
	Parser struct {
		MaxMetadataSize     int64
		MaxCompressedSize   int64
		MaxDecompressedSize int64
		MaxBlocks           uint32
		EventDecoders       []EventDecoder
	}
)

// NewParser creates a parser with conservative limits suitable for uploaded replays.
func NewParser() (parser *Parser) {
	parser = &Parser{
		MaxMetadataSize:     defaultMaxMetadataSize,
		MaxCompressedSize:   defaultMaxCompressedSize,
		MaxDecompressedSize: defaultMaxDecompressedSize,
		MaxBlocks:           defaultMaxBlocks,
	}

	return
}

// Parse reads and decodes one replay from a byte stream.
func Parse(reader io.Reader) (replay *Replay, err error) {
	replay, err = NewParser().Parse(reader)

	return
}

// ParseBytes decodes one replay from an in-memory byte slice.
func ParseBytes(data []byte) (replay *Replay, err error) {
	replay, err = Parse(bytes.NewReader(data))

	return
}

// ParseFile opens and decodes one replay at path.
func ParseFile(path string) (replay *Replay, err error) {
	var file *os.File

	if file, err = os.Open(path); err != nil {
		err = fmt.Errorf("open replay %q: %w", path, err)

		return
	}
	defer file.Close()

	if replay, err = Parse(file); err != nil {
		err = fmt.Errorf("parse replay %q: %w", path, err)
	}

	return
}

// Parse reads the container, decrypts and decompresses packets, and decodes stable event types.
func (parser *Parser) Parse(reader io.Reader) (replay *Replay, err error) {
	var (
		blockCount       uint32
		compressedSize   uint32
		decompressedSize uint32
		encrypted        []byte
		magic            uint32
		metadataBytes    []byte
		packetData       []byte
	)

	if parser == nil {
		parser = NewParser()
	} else {
		if parser.MaxMetadataSize == 0 {
			parser.MaxMetadataSize = defaultMaxMetadataSize
		}

		if parser.MaxCompressedSize == 0 {
			parser.MaxCompressedSize = defaultMaxCompressedSize
		}

		if parser.MaxDecompressedSize == 0 {
			parser.MaxDecompressedSize = defaultMaxDecompressedSize
		}

		if parser.MaxBlocks == 0 {
			parser.MaxBlocks = defaultMaxBlocks
		}
	}

	if err = readUint32(reader, &magic); err != nil {
		return
	}

	if magic != replayMagic {
		err = fmt.Errorf("%w: got 0x%08x", ErrInvalidMagic, magic)

		return
	}

	if err = readUint32(reader, &blockCount); err != nil {
		return
	}

	if blockCount == 0 || blockCount > parser.MaxBlocks {
		err = fmt.Errorf("%w: block count %d is outside 1..%d", ErrInvalidContainer, blockCount, parser.MaxBlocks)

		return
	}

	if metadataBytes, err = readSizedBlock(reader, parser.MaxMetadataSize); err != nil {
		err = fmt.Errorf("read metadata: %w", err)

		return
	}

	replay = &Replay{}
	if err = json.Unmarshal(metadataBytes, &replay.Metadata); err != nil {
		err = fmt.Errorf("decode metadata JSON: %w", err)

		return
	}

	replay.ExtraBlocks = make([][]byte, 0, blockCount-1)
	for blockIndex := uint32(1); blockIndex < blockCount; blockIndex++ {
		var block []byte

		if block, err = readSizedBlock(reader, parser.MaxMetadataSize); err != nil {
			err = fmt.Errorf("read metadata block %d: %w", blockIndex, err)

			return
		}

		replay.ExtraBlocks = append(replay.ExtraBlocks, block)
	}

	if err = readUint32(reader, &decompressedSize); err != nil {
		return
	}

	if err = readUint32(reader, &compressedSize); err != nil {
		return
	}

	if int64(compressedSize) > parser.MaxCompressedSize || int64(decompressedSize) > parser.MaxDecompressedSize {
		err = ErrLimitExceeded

		return
	}

	if encrypted, err = io.ReadAll(io.LimitReader(reader, parser.MaxCompressedSize+1)); err != nil {
		err = fmt.Errorf("read encrypted packet stream: %w", err)

		return
	}

	if int64(len(encrypted)) > parser.MaxCompressedSize {
		err = ErrLimitExceeded

		return
	}

	if len(encrypted)%blowfish.BlockSize != 0 {
		err = fmt.Errorf("%w: encrypted stream size %d is not block aligned", ErrInvalidContainer, len(encrypted))

		return
	}

	if packetData, err = decryptAndInflate(encrypted, int64(decompressedSize), parser.MaxDecompressedSize); err != nil {
		return
	}

	replay.packetData = packetData
	replay.Stats.CompressedSize = compressedSize
	replay.Stats.DecompressedSize = decompressedSize

	if uint32(len(packetData)) != decompressedSize {
		replay.Warnings = append(replay.Warnings, fmt.Sprintf(
			"container declares %d decompressed bytes; decoded %d",
			decompressedSize,
			len(packetData),
		))
	}

	if compressedSize != 0 && int(compressedSize) > len(encrypted) {
		replay.Warnings = append(replay.Warnings, fmt.Sprintf(
			"container declares %d compressed bytes; encrypted stream has %d",
			compressedSize,
			len(encrypted),
		))
	}

	if replay.Events, err = decodeEvents(packetData, replay.Metadata.ClientVersionFromExe); err != nil {
		return
	}

	for eventIndex := range replay.Events {
		for _, decoder := range parser.EventDecoders {
			var (
				data    any
				handled bool
			)

			if data, handled, err = decoder.DecodeEvent(replay.Metadata, replay.Events[eventIndex]); err != nil {
				err = fmt.Errorf("decode event %d with semantic decoder: %w", eventIndex, err)

				return
			}

			if handled {
				replay.Events[eventIndex].Data = data

				break
			}
		}
	}

	replay.Stats.PacketCount = len(replay.Events)

	return
}

// readSizedBlock reads one length-prefixed replay metadata block.
func readSizedBlock(reader io.Reader, maximum int64) (block []byte, err error) {
	var size uint32

	if err = readUint32(reader, &size); err != nil {
		return
	}

	if int64(size) > maximum {
		err = ErrLimitExceeded

		return
	}

	block = make([]byte, size)
	if _, err = io.ReadFull(reader, block); err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidContainer, err)
	}

	return
}

// readUint32 reads a little-endian integer from the replay stream.
func readUint32(reader io.Reader, value *uint32) (err error) {
	if err = binary.Read(reader, binary.LittleEndian, value); err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidContainer, err)
	}

	return
}

// decryptAndInflate applies the WoWS Blowfish chain and zlib decompression.
func decryptAndInflate(encrypted []byte, declaredSize, maximumSize int64) (packetData []byte, err error) {
	var (
		cipherBlock *blowfish.Cipher
		decoded     []byte = make([]byte, len(encrypted))
		previous    [blowfish.BlockSize]byte
		zlibReader  io.ReadCloser
	)

	if cipherBlock, err = blowfish.NewCipher(replayKey[:]); err != nil {
		return
	}

	for offset := 0; offset < len(encrypted); offset += blowfish.BlockSize {
		cipherBlock.Decrypt(decoded[offset:offset+blowfish.BlockSize], encrypted[offset:offset+blowfish.BlockSize])

		for byteIndex := 0; byteIndex < blowfish.BlockSize; byteIndex++ {
			decoded[offset+byteIndex] ^= previous[byteIndex]
		}

		copy(previous[:], decoded[offset:offset+blowfish.BlockSize])
	}

	if zlibReader, err = zlib.NewReader(bytes.NewReader(decoded)); err != nil {
		err = fmt.Errorf("open replay zlib stream: %w", err)

		return
	}
	defer zlibReader.Close()

	if declaredSize > 0 && declaredSize < maximumSize {
		packetData = make([]byte, 0, declaredSize)
	}

	if packetData, err = io.ReadAll(io.LimitReader(zlibReader, maximumSize+1)); err != nil {
		err = fmt.Errorf("decompress replay packets: %w", err)

		return
	}

	if int64(len(packetData)) > maximumSize {
		packetData = nil
		err = ErrLimitExceeded
	}

	return
}
