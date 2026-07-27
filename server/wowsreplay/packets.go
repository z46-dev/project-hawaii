package wowsreplay

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	packetHeaderSize int = 12
)

type (
	packetLayout struct {
		names map[uint32]string
	}

	payloadReader struct {
		data   []byte
		offset int
	}
)

// decodeEvents walks the definition-independent packet stream without dropping unknown packets.
func decodeEvents(packetData []byte, version string) (events []Event, err error) {
	var (
		layout    packetLayout = layoutForVersion(version)
		remaining []byte       = packetData
	)

	events = make([]Event, 0, len(packetData)/64)

	for len(remaining) > 0 {
		var (
			clock      float32
			event      Event
			packetSize uint32
			packetType uint32
			payload    []byte
		)

		if len(remaining) < packetHeaderSize {
			err = fmt.Errorf("%w: %d bytes remain before packet %d", ErrTruncatedPacket, len(remaining), len(events))

			return
		}

		packetSize = binary.LittleEndian.Uint32(remaining[0:4])
		packetType = binary.LittleEndian.Uint32(remaining[4:8])
		clock = math.Float32frombits(binary.LittleEndian.Uint32(remaining[8:12]))
		remaining = remaining[packetHeaderSize:]

		if uint64(packetSize) > uint64(len(remaining)) {
			err = fmt.Errorf(
				"%w: packet %d declares %d bytes; %d remain",
				ErrTruncatedPacket,
				len(events),
				packetSize,
				len(remaining),
			)

			return
		}

		payload = remaining[:packetSize]
		remaining = remaining[packetSize:]
		event = Event{
			Index:      len(events),
			Clock:      clock,
			TypeID:     packetType,
			Type:       layout.name(packetType),
			RawPayload: payload,
		}
		event.Data = decodePayload(event.Type, payload)
		events = append(events, event)
	}

	return
}

// decodePayload parses packet types whose structure is stable without game entity definitions.
func decodePayload(packetType string, payload []byte) (value any) {
	var reader payloadReader = payloadReader{data: payload}

	switch packetType {
	case "Position":
		value = decodePosition(&reader)
	case "NonVolatilePosition":
		value = decodeNonVolatilePosition(&reader)
	case "EntityCreate":
		value = decodeEntityCreate(&reader)
	case "CellPlayerCreate":
		value = decodeCellPlayerCreate(&reader)
	case "EntityEnter":
		value = decodeEntityEnter(&reader)
	case "EntityLeave", "OwnShip":
		value = decodeEntityReference(&reader)
	case "EntityControl":
		value = decodeEntityControl(&reader)
	case "PlayerNetStats":
		value = decodeNetworkStats(&reader)
	case "Version":
		value = reader.string32()
	case "ServerTick", "ServerTimestamp":
		value = reader.float64()
	case "InitFlag", "CameraFreeLook":
		value = reader.uint8()
	case "CameraMode":
		value = reader.uint32()
	case "Camera":
		value = decodeCamera(&reader)
	case "PlayerOrientation":
		value = decodePlayerOrientation(&reader)
	case "SetWeaponLock":
		value = decodeWeaponLock(&reader)
	case "SubController":
		value = reader.int16()
	case "CruiseState":
		value = map[string]any{"key": reader.uint32(), "value": reader.int32()}
	case "ShotTracking":
		value = ShotTrackingData{EntityID: reader.uint32(), Value: reader.int64()}
	case "BattleResults":
		value = reader.string32()
	case "GunMarker":
		value = decodeGunMarker(&reader)
	case "Map":
		value = decodeMap(&reader)
	}

	if reader.offset > len(reader.data) {
		value = nil
	}

	return
}

// decodeNonVolatilePosition decodes a reliable position and rotation update.
func decodeNonVolatilePosition(reader *payloadReader) (position NonVolatilePositionData) {
	position = NonVolatilePositionData{
		EntityID: reader.uint32(),
		SpaceID:  reader.uint32(),
		Position: reader.vector3(),
		Rotation: reader.rotation(),
	}

	return
}

// decodeGunMarker decodes the current aiming marker.
func decodeGunMarker(reader *payloadReader) (marker GunMarkerData) {
	marker = GunMarkerData{
		TargetPoint:      reader.vector3(),
		Diameter:         reader.float32(),
		MarkerPosition:   reader.vector3(),
		MarkerDirection:  reader.vector3(),
		ArcadeMarkerSize: reader.float32(),
	}
	marker.SPGMarkerParams[0] = reader.float32()
	marker.SPGMarkerParams[1] = reader.float32()

	return
}

// decodeMap decodes the replay space and arena identity packet.
func decodeMap(reader *payloadReader) (mapData MapData) {
	var err error

	mapData.SpaceID = reader.uint32()
	mapData.ArenaID = reader.int64()
	mapData.Unknown1 = reader.uint32()
	mapData.Unknown2 = reader.uint32()
	if mapData.Blob, err = reader.take(128); err != nil {
		return
	}
	mapData.MapName = reader.string32()
	if mapData.Matrix, err = reader.take(64); err != nil {
		return
	}
	mapData.Unknown = reader.uint8()

	return
}

// decodePosition decodes a volatile movement packet.
func decodePosition(reader *payloadReader) (position PositionData) {
	position = PositionData{
		EntityID:  reader.uint32(),
		SpaceID:   reader.uint32(),
		Position:  reader.vector3(),
		Direction: reader.vector3(),
		Rotation:  reader.rotation(),
		OnGround:  reader.uint8() != 0,
	}

	return
}

// decodeEntityCreate decodes the stable prefix before version-specific entity state.
func decodeEntityCreate(reader *payloadReader) (entity EntityCreateData) {
	entity = EntityCreateData{
		EntityID:   reader.uint32(),
		EntityType: reader.uint16(),
		VehicleID:  reader.uint32(),
		SpaceID:    reader.uint32(),
		Position:   reader.vector3(),
		Rotation:   reader.rotation(),
		StateSize:  reader.uint32(),
	}

	return
}

// decodeCellPlayerCreate decodes the stable prefix before version-specific player state.
func decodeCellPlayerCreate(reader *payloadReader) (entity CellPlayerCreateData) {
	entity = CellPlayerCreateData{
		EntityID:  reader.uint32(),
		SpaceID:   reader.uint32(),
		VehicleID: reader.uint32(),
		Position:  reader.vector3(),
		Rotation:  reader.rotation(),
		StateSize: reader.uint32(),
	}

	return
}

// decodeEntityEnter decodes an entity entering a replay space.
func decodeEntityEnter(reader *payloadReader) (entity EntityEnterData) {
	entity = EntityEnterData{
		EntityID:  reader.uint32(),
		SpaceID:   reader.uint32(),
		VehicleID: reader.uint32(),
	}

	return
}

// decodeEntityReference decodes a packet containing one entity identifier.
func decodeEntityReference(reader *payloadReader) (entity EntityReferenceData) {
	entity.EntityID = reader.uint32()

	return
}

// decodeEntityControl decodes an entity-control update.
func decodeEntityControl(reader *payloadReader) (control EntityControlData) {
	control = EntityControlData{EntityID: reader.uint32(), Controlled: reader.uint8() != 0}

	return
}

// decodeNetworkStats decodes the packed client performance integer.
func decodeNetworkStats(reader *payloadReader) (stats NetworkStatsData) {
	var packed uint32 = reader.uint32()

	stats = NetworkStatsData{
		FPS:       uint8(packed & 0xff),
		Ping:      uint16((packed >> 8) & 0xffff),
		IsLagging: packed&(1<<24) != 0,
	}

	return
}

// decodeCamera decodes modern and legacy camera packet layouts.
func decodeCamera(reader *payloadReader) (camera CameraData) {
	for index := range camera.Quaternion {
		camera.Quaternion[index] = reader.float32()
	}

	camera.CameraPosition = reader.vector3()
	camera.FieldOfView = reader.float32()

	if len(reader.data) >= 60 {
		reader.float32()
	}

	camera.Position = reader.vector3()
	camera.Direction = reader.vector3()

	return
}

// decodePlayerOrientation decodes the player/camera orientation packet.
func decodePlayerOrientation(reader *payloadReader) (orientation PlayerOrientationData) {
	orientation = PlayerOrientationData{
		EntityID: reader.uint32(),
		ParentID: reader.uint32(),
		Position: reader.vector3(),
		Rotation: reader.rotation(),
	}

	return
}

// decodeWeaponLock decodes one weapon-lock update.
func decodeWeaponLock(reader *payloadReader) (lock WeaponLockData) {
	lock = WeaponLockData{
		WeaponType: reader.uint32(),
		LockType:   reader.uint32(),
		TargetID:   reader.uint32(),
	}

	return
}

// layoutForVersion chooses the packet identifiers used before and after WoWS 12.6.
func layoutForVersion(version string) (layout packetLayout) {
	var modern bool = versionAtLeast(version, 12, 6, 0)

	layout.names = map[uint32]string{
		0x00:       "BasePlayerCreate",
		0x01:       "CellPlayerCreate",
		0x02:       "EntityControl",
		0x03:       "EntityEnter",
		0x04:       "EntityLeave",
		0x05:       "EntityCreate",
		0x07:       "EntityProperty",
		0x08:       "EntityMethod",
		0x0a:       "Position",
		0x0e:       "ServerTick",
		0x0f:       "ServerTimestamp",
		0x10:       "InitFlag",
		0x13:       "InitMarker",
		0x16:       "Version",
		0x18:       "GunMarker",
		0x1d:       "PlayerNetStats",
		0x20:       "OwnShip",
		0x26:       "BasePlayerCreateStub",
		0xffffffff: "StreamEnd",
	}

	if modern {
		layout.names[0x22] = "BattleResults"
		layout.names[0x23] = "NestedPropertyUpdate"
		layout.names[0x25] = "Camera"
		layout.names[0x27] = "CameraMode"
		layout.names[0x28] = "Map"
		layout.names[0x2a] = "NonVolatilePosition"
		layout.names[0x2c] = "PlayerOrientation"
		layout.names[0x2f] = "CameraFreeLook"
		layout.names[0x30] = "SetWeaponLock"
		layout.names[0x31] = "SubController"
		layout.names[0x32] = "CruiseState"
		layout.names[0x33] = "ShotTracking"
	} else {
		layout.names[0x22] = "NestedPropertyUpdate"
		layout.names[0x24] = "Camera"
		layout.names[0x27] = "Map"
		layout.names[0x29] = "NonVolatilePosition"
		layout.names[0x2b] = "PlayerOrientation"
		layout.names[0x2e] = "CameraFreeLook"
		layout.names[0x2f] = "SetWeaponLock"
		layout.names[0x30] = "SubController"
		layout.names[0x31] = "CruiseState"
	}

	return
}

// name returns a stable name while retaining unknown numeric identifiers.
func (layout packetLayout) name(identifier uint32) (name string) {
	var found bool

	if name, found = layout.names[identifier]; !found {
		name = fmt.Sprintf("Unknown_0x%02x", identifier)
	}

	return
}

// versionAtLeast compares the first three numeric components of a WoWS version string.
func versionAtLeast(version string, major, minor, patch int) (result bool) {
	var (
		components []string = strings.FieldsFunc(version, func(character rune) bool {
			return character < '0' || character > '9'
		})
		values [3]int
	)

	for index := 0; index < len(values) && index < len(components); index++ {
		values[index], _ = strconv.Atoi(components[index])
	}

	if values[0] != major {
		result = values[0] > major
	} else if values[1] != minor {
		result = values[1] > minor
	} else {
		result = values[2] >= patch
	}

	return
}

// vector3 reads three little-endian float32 values.
func (reader *payloadReader) vector3() (vector Vector3) {
	vector = Vector3{X: reader.float32(), Y: reader.float32(), Z: reader.float32()}

	return
}

// rotation reads yaw, pitch, and roll.
func (reader *payloadReader) rotation() (rotation Rotation) {
	rotation = Rotation{Yaw: reader.float32(), Pitch: reader.float32(), Roll: reader.float32()}

	return
}

// uint8 reads an unsigned byte and records underflow.
func (reader *payloadReader) uint8() (value uint8) {
	if reader.offset+1 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = reader.data[reader.offset]
	reader.offset++

	return
}

// uint16 reads a little-endian unsigned integer and records underflow.
func (reader *payloadReader) uint16() (value uint16) {
	if reader.offset+2 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = binary.LittleEndian.Uint16(reader.data[reader.offset:])
	reader.offset += 2

	return
}

// int16 reads a little-endian signed integer and records underflow.
func (reader *payloadReader) int16() (value int16) {
	value = int16(reader.uint16())

	return
}

// uint32 reads a little-endian unsigned integer and records underflow.
func (reader *payloadReader) uint32() (value uint32) {
	if reader.offset+4 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = binary.LittleEndian.Uint32(reader.data[reader.offset:])
	reader.offset += 4

	return
}

// int32 reads a little-endian signed integer and records underflow.
func (reader *payloadReader) int32() (value int32) {
	value = int32(reader.uint32())

	return
}

// int64 reads a little-endian signed integer and records underflow.
func (reader *payloadReader) int64() (value int64) {
	if reader.offset+8 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = int64(binary.LittleEndian.Uint64(reader.data[reader.offset:]))
	reader.offset += 8

	return
}

// float32 reads a little-endian IEEE-754 number.
func (reader *payloadReader) float32() (value float32) {
	value = math.Float32frombits(reader.uint32())

	return
}

// float64 reads a little-endian IEEE-754 number.
func (reader *payloadReader) float64() (value float64) {
	if reader.offset+8 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = math.Float64frombits(binary.LittleEndian.Uint64(reader.data[reader.offset:]))
	reader.offset += 8

	return
}

// string32 reads a little-endian length-prefixed UTF-8 string.
func (reader *payloadReader) string32() (value string) {
	var length uint32 = reader.uint32()

	if reader.offset+int(length) > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = string(reader.data[reader.offset : reader.offset+int(length)])
	reader.offset += int(length)

	return
}
