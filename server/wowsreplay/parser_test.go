package wowsreplay

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/blowfish"
)

// TestParseBytes verifies the complete container, crypto, compression, and packet pipeline.
func TestParseBytes(t *testing.T) {
	var (
		packetData bytes.Buffer
		position   bytes.Buffer
		replay     *Replay
		err        error
	)

	writeUint32Test(t, &position, 42)
	writeUint32Test(t, &position, 0)
	writeFloat32Test(t, &position, 100)
	writeFloat32Test(t, &position, 1)
	writeFloat32Test(t, &position, -200)
	writeFloat32Test(t, &position, 2)
	writeFloat32Test(t, &position, 0)
	writeFloat32Test(t, &position, 3)
	writeFloat32Test(t, &position, 0.5)
	writeFloat32Test(t, &position, 0)
	writeFloat32Test(t, &position, 0)
	position.WriteByte(1)

	writePacketTest(t, &packetData, 0x16, 0, lengthPrefixedStringTest(t, "15.1.0"))
	writePacketTest(t, &packetData, 0x20, 1, uint32BytesTest(t, 42))
	writePacketTest(t, &packetData, 0x0a, 2, position.Bytes())
	writePacketTest(t, &packetData, 0x99, 3, []byte{1, 2, 3})

	if replay, err = ParseBytes(buildReplayTest(t, packetData.Bytes())); err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	if replay.Metadata.PlayerName != "Parser Test" {
		t.Fatalf("PlayerName = %q, want Parser Test", replay.Metadata.PlayerName)
	}

	if len(replay.Events) != 4 {
		t.Fatalf("event count = %d, want 4", len(replay.Events))
	}

	if replay.Events[0].Data != "15.1.0" {
		t.Fatalf("version data = %#v, want 15.1.0", replay.Events[0].Data)
	}

	var parsedPosition PositionData
	var valid bool

	if parsedPosition, valid = replay.Events[2].Data.(PositionData); !valid {
		t.Fatalf("position data type = %T", replay.Events[2].Data)
	}

	if parsedPosition.EntityID != 42 || parsedPosition.Position.X != 100 || parsedPosition.Position.Z != -200 {
		t.Fatalf("position data = %#v", parsedPosition)
	}

	if replay.Events[3].Type != "Unknown_0x99" || !bytes.Equal(replay.Events[3].RawPayload, []byte{1, 2, 3}) {
		t.Fatalf("unknown event = %#v", replay.Events[3])
	}
}

// TestWriteEventLog verifies machine-readable and human-readable output.
func TestWriteEventLog(t *testing.T) {
	var (
		output bytes.Buffer
		replay Replay = Replay{
			Metadata: Metadata{PlayerName: "Logger"},
			Events: []Event{{
				Index:      0,
				Clock:      1.5,
				TypeID:     0x20,
				Type:       "OwnShip",
				Data:       EntityReferenceData{EntityID: 7},
				RawPayload: []byte{7, 0, 0, 0},
			}},
			Stats: ContainerStats{PacketCount: 1},
		}
		err error
	)

	if err = replay.WriteEventLog(&output, LogOptions{Format: LogFormatJSONLines}); err != nil {
		t.Fatalf("WriteEventLog(JSONL) error = %v", err)
	}

	if !strings.Contains(output.String(), `"record":"event"`) || strings.Contains(output.String(), "rawPayload") {
		t.Fatalf("unexpected JSONL output: %s", output.String())
	}

	output.Reset()
	if err = replay.WriteEventLog(&output, LogOptions{Format: LogFormatText, IncludePayload: true}); err != nil {
		t.Fatalf("WriteEventLog(text) error = %v", err)
	}

	if !strings.Contains(output.String(), "OwnShip") || !strings.Contains(output.String(), "raw=07000000") {
		t.Fatalf("unexpected text output: %s", output.String())
	}
}

// TestWriteVideo verifies that the ffmpeg streaming path produces an MP4.
func TestWriteVideo(t *testing.T) {
	var (
		lookupErr error
		output    bytes.Buffer
		replay    Replay
		err       error
	)

	if _, lookupErr = exec.LookPath("ffmpeg"); lookupErr != nil {
		t.Skip("ffmpeg is not installed")
	}

	for index := 0; index < 5; index++ {
		replay.Events = append(replay.Events, Event{
			Index: index,
			Clock: float32(index),
			Type:  "Position",
			Data: PositionData{
				EntityID: 1,
				Position: Vector3{X: float32(index * 100), Z: float32(index * 50)},
			},
		})
	}

	if err = replay.WriteVideo(context.Background(), &output, VideoOptions{
		Width:           64,
		Height:          64,
		FramesPerSecond: 1,
		PlaybackSpeed:   4,
		CRF:             35,
		MaxEntities:     4,
	}); err != nil {
		t.Fatalf("WriteVideo() error = %v", err)
	}

	if !bytes.Contains(output.Bytes(), []byte("ftyp")) {
		t.Fatal("video output does not contain an MP4 ftyp box")
	}
}

// TestExternalReplay validates an optional real replay supplied by a developer.
func TestExternalReplay(t *testing.T) {
	var path string = os.Getenv("WOWS_REPLAY_TEST_FILE")

	if path == "" {
		t.Skip("WOWS_REPLAY_TEST_FILE is not set")
	}

	var (
		replay *Replay
		err    error
	)

	if replay, err = ParseFile(path); err != nil {
		t.Fatalf("ParseFile(%q) error = %v", path, err)
	}

	if len(replay.Events) == 0 {
		t.Fatal("real replay decoded no events")
	}

	var (
		ownShipID uint32
		samples   []positionSample
	)

	samples, ownShipID = replay.BuildMatch().positionSamples()
	if len(samples) == 0 {
		t.Fatal("real replay decoded no position samples")
	}

	t.Logf(
		"decoded %d packets and %d positions from WoWS %s on %s; own ship %d",
		len(replay.Events),
		len(samples),
		replay.Metadata.ClientVersionFromExe,
		replay.Metadata.MapDisplayName,
		ownShipID,
	)
}

// buildReplayTest creates a valid encrypted replay container for unit tests.
func buildReplayTest(t *testing.T, packetData []byte) (container []byte) {
	var (
		compressed bytes.Buffer
		writer     *zlib.Writer
		encrypted  []byte
		output     bytes.Buffer
		metadata   []byte = []byte(`{
			"clientVersionFromExe":"15, 1, 0, 0",
			"playerName":"Parser Test",
			"mapDisplayName":"Test Map",
			"duration":120,
			"futureMetadataField":{"enabled":true}
		}`)
		writeErr error
	)

	writer = zlib.NewWriter(&compressed)
	if _, writeErr = writer.Write(packetData); writeErr != nil {
		t.Fatalf("compress packets: %v", writeErr)
	}

	if writeErr = writer.Close(); writeErr != nil {
		t.Fatalf("close compressor: %v", writeErr)
	}

	for compressed.Len()%blowfish.BlockSize != 0 {
		compressed.WriteByte(0)
	}

	encrypted = encryptReplayTest(t, compressed.Bytes())
	writeUint32Test(t, &output, replayMagic)
	writeUint32Test(t, &output, 1)
	writeUint32Test(t, &output, uint32(len(metadata)))
	output.Write(metadata)
	writeUint32Test(t, &output, uint32(len(packetData)))
	writeUint32Test(t, &output, uint32(compressed.Len()))
	output.Write(encrypted)
	container = output.Bytes()

	return
}

// encryptReplayTest applies the inverse of the replay's plaintext-chained decryptor.
func encryptReplayTest(t *testing.T, plaintext []byte) (encrypted []byte) {
	var (
		cipherBlock *blowfish.Cipher
		err         error
		previous    [blowfish.BlockSize]byte
		input       [blowfish.BlockSize]byte
	)

	if cipherBlock, err = blowfish.NewCipher(replayKey[:]); err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	encrypted = make([]byte, len(plaintext))
	for offset := 0; offset < len(plaintext); offset += blowfish.BlockSize {
		for index := 0; index < blowfish.BlockSize; index++ {
			input[index] = plaintext[offset+index] ^ previous[index]
		}

		cipherBlock.Encrypt(encrypted[offset:offset+blowfish.BlockSize], input[:])
		copy(previous[:], plaintext[offset:offset+blowfish.BlockSize])
	}

	return
}

// writePacketTest serializes one replay packet.
func writePacketTest(t *testing.T, writer io.Writer, packetType uint32, clock float32, payload []byte) {
	var err error

	writeUint32Test(t, writer, uint32(len(payload)))
	writeUint32Test(t, writer, packetType)
	writeFloat32Test(t, writer, clock)

	if _, err = writer.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
}

// uint32BytesTest serializes a little-endian integer.
func uint32BytesTest(t *testing.T, value uint32) (data []byte) {
	var output bytes.Buffer

	writeUint32Test(t, &output, value)
	data = output.Bytes()

	return
}

// lengthPrefixedStringTest serializes a replay protocol string.
func lengthPrefixedStringTest(t *testing.T, value string) (data []byte) {
	var output bytes.Buffer

	writeUint32Test(t, &output, uint32(len(value)))
	output.WriteString(value)
	data = output.Bytes()

	return
}

// writeUint32Test writes a little-endian integer or fails the test.
func writeUint32Test(t *testing.T, writer io.Writer, value uint32) {
	var err error

	if err = binary.Write(writer, binary.LittleEndian, value); err != nil {
		t.Fatalf("write uint32: %v", err)
	}
}

// writeFloat32Test writes a little-endian float or fails the test.
func writeFloat32Test(t *testing.T, writer io.Writer, value float32) {
	var err error

	if err = binary.Write(writer, binary.LittleEndian, value); err != nil {
		t.Fatalf("write float32: %v", err)
	}
}
