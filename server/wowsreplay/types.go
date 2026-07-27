package wowsreplay

import (
	"encoding/json"
	"fmt"
)

const (
	replayMagic uint32 = 0x11343212
)

type (
	// Metadata contains the stable fields from the replay's plaintext JSON header.
	Metadata struct {
		MatchGroup           string                     `json:"matchGroup,omitempty"`
		GameMode             uint32                     `json:"gameMode,omitempty"`
		GameType             string                     `json:"gameType,omitempty"`
		ClientVersionFromExe string                     `json:"clientVersionFromExe,omitempty"`
		MapDisplayName       string                     `json:"mapDisplayName,omitempty"`
		MapID                uint32                     `json:"mapId,omitempty"`
		ClientVersionFromXML string                     `json:"clientVersionFromXml,omitempty"`
		Duration             uint32                     `json:"duration,omitempty"`
		Name                 string                     `json:"name,omitempty"`
		Scenario             string                     `json:"scenario,omitempty"`
		PlayerID             int64                      `json:"playerID,omitempty"`
		Vehicles             []Vehicle                  `json:"vehicles,omitempty"`
		PlayersPerTeam       uint32                     `json:"playersPerTeam,omitempty"`
		DateTime             string                     `json:"dateTime,omitempty"`
		MapName              string                     `json:"mapName,omitempty"`
		PlayerName           string                     `json:"playerName,omitempty"`
		ScenarioConfigID     uint32                     `json:"scenarioConfigId,omitempty"`
		TeamsCount           uint32                     `json:"teamsCount,omitempty"`
		PlayerVehicle        string                     `json:"playerVehicle,omitempty"`
		BattleDuration       uint32                     `json:"battleDuration,omitempty"`
		Raw                  map[string]json.RawMessage `json:"-"`
	}

	// Vehicle describes a participant listed in the replay metadata.
	Vehicle struct {
		ShipID   uint64 `json:"shipId"`
		Relation uint32 `json:"relation"`
		ID       int64  `json:"id"`
		Name     string `json:"name"`
	}

	// Replay is the decoded replay metadata and chronological event stream.
	Replay struct {
		Metadata    Metadata       `json:"metadata"`
		ExtraBlocks [][]byte       `json:"extraBlocks,omitempty"`
		Events      []Event        `json:"events"`
		Stats       ContainerStats `json:"stats"`
		Warnings    []string       `json:"warnings,omitempty"`
		packetData  []byte
	}

	// ContainerStats reports sizes and counts from the replay container.
	ContainerStats struct {
		CompressedSize   uint32 `json:"compressedSize"`
		DecompressedSize uint32 `json:"decompressedSize"`
		PacketCount      int    `json:"packetCount"`
	}

	// Event is one replay packet with stable fields decoded into Data.
	Event struct {
		Index      int     `json:"index"`
		Clock      float32 `json:"clock"`
		TypeID     uint32  `json:"typeId"`
		Type       string  `json:"type"`
		Data       any     `json:"data,omitempty"`
		RawPayload []byte  `json:"rawPayload,omitempty"`
	}

	// EventDecoder adds version-specific semantic decoding without changing container parsing.
	EventDecoder interface {
		DecodeEvent(metadata Metadata, event Event) (data any, handled bool, err error)
	}

	// Vector3 is a three-dimensional BigWorld coordinate or direction.
	Vector3 struct {
		X float32 `json:"x"`
		Y float32 `json:"y"`
		Z float32 `json:"z"`
	}

	// Rotation is a BigWorld yaw, pitch, and roll.
	Rotation struct {
		Yaw   float32 `json:"yaw"`
		Pitch float32 `json:"pitch"`
		Roll  float32 `json:"roll"`
	}

	// PositionData is a volatile entity movement update.
	PositionData struct {
		EntityID  uint32   `json:"entityId"`
		SpaceID   uint32   `json:"spaceId"`
		Position  Vector3  `json:"position"`
		Direction Vector3  `json:"direction"`
		Rotation  Rotation `json:"rotation"`
		OnGround  bool     `json:"onGround"`
	}

	// NonVolatilePositionData is a reliable entity movement and orientation update.
	NonVolatilePositionData struct {
		EntityID uint32   `json:"entityId"`
		SpaceID  uint32   `json:"spaceId"`
		Position Vector3  `json:"position"`
		Rotation Rotation `json:"rotation"`
	}

	// EntityCreateData contains the definition-independent EntityCreate prefix.
	EntityCreateData struct {
		EntityID   uint32   `json:"entityId"`
		EntityType uint16   `json:"entityType"`
		VehicleID  uint32   `json:"vehicleId"`
		SpaceID    uint32   `json:"spaceId"`
		Position   Vector3  `json:"position"`
		Rotation   Rotation `json:"rotation"`
		StateSize  uint32   `json:"stateSize"`
	}

	// CellPlayerCreateData contains the definition-independent CellPlayerCreate prefix.
	CellPlayerCreateData struct {
		EntityID  uint32   `json:"entityId"`
		SpaceID   uint32   `json:"spaceId"`
		VehicleID uint32   `json:"vehicleId"`
		Position  Vector3  `json:"position"`
		Rotation  Rotation `json:"rotation"`
		StateSize uint32   `json:"stateSize"`
	}

	// EntityReferenceData identifies an entity referenced by a packet.
	EntityReferenceData struct {
		EntityID uint32 `json:"entityId"`
	}

	// EntityEnterData records an entity entering a space.
	EntityEnterData struct {
		EntityID  uint32 `json:"entityId"`
		SpaceID   uint32 `json:"spaceId"`
		VehicleID uint32 `json:"vehicleId"`
	}

	// EntityControlData records client control of an entity.
	EntityControlData struct {
		EntityID   uint32 `json:"entityId"`
		Controlled bool   `json:"controlled"`
	}

	// NetworkStatsData contains client performance data embedded in a replay.
	NetworkStatsData struct {
		FPS       uint8  `json:"fps"`
		Ping      uint16 `json:"ping"`
		IsLagging bool   `json:"isLagging"`
	}

	// CameraData contains the replay camera state.
	CameraData struct {
		Quaternion     [4]float32 `json:"quaternion"`
		CameraPosition Vector3    `json:"cameraPosition"`
		FieldOfView    float32    `json:"fieldOfView"`
		Position       Vector3    `json:"position"`
		Direction      Vector3    `json:"direction"`
	}

	// PlayerOrientationData contains the player or camera orientation update.
	PlayerOrientationData struct {
		EntityID uint32   `json:"entityId"`
		ParentID uint32   `json:"parentId"`
		Position Vector3  `json:"position"`
		Rotation Rotation `json:"rotation"`
	}

	// WeaponLockData records a weapon target-lock state change.
	WeaponLockData struct {
		WeaponType uint32 `json:"weaponType"`
		LockType   uint32 `json:"lockType"`
		TargetID   uint32 `json:"targetId"`
	}

	// ShotTrackingData records a shot-tracking counter update.
	ShotTrackingData struct {
		EntityID uint32 `json:"entityId"`
		Value    int64  `json:"value"`
	}

	// GunMarkerData describes the player's current aiming solution.
	GunMarkerData struct {
		TargetPoint      Vector3    `json:"targetPoint"`
		Diameter         float32    `json:"diameter"`
		MarkerPosition   Vector3    `json:"markerPosition"`
		MarkerDirection  Vector3    `json:"markerDirection"`
		ArcadeMarkerSize float32    `json:"arcadeMarkerSize"`
		SPGMarkerParams  [2]float32 `json:"spgMarkerParams"`
	}

	// MapData identifies the arena and map loaded by the replay space.
	MapData struct {
		SpaceID  uint32 `json:"spaceId"`
		ArenaID  int64  `json:"arenaId"`
		Unknown1 uint32 `json:"unknown1"`
		Unknown2 uint32 `json:"unknown2"`
		Blob     []byte `json:"blob"`
		MapName  string `json:"mapName"`
		Matrix   []byte `json:"matrix"`
		Unknown  uint8  `json:"unknown"`
	}
)

// UnmarshalJSON preserves both stable metadata fields and unrecognized version-specific fields.
func (metadata *Metadata) UnmarshalJSON(data []byte) (err error) {
	type metadataAlias Metadata

	var (
		alias metadataAlias
		raw   map[string]json.RawMessage
	)

	if err = json.Unmarshal(data, &alias); err != nil {
		return
	}

	if err = json.Unmarshal(data, &raw); err != nil {
		return
	}

	*metadata = Metadata(alias)
	metadata.Raw = raw

	return
}

// MarshalJSON emits stable fields together with untouched version-specific metadata.
func (metadata Metadata) MarshalJSON() (data []byte, err error) {
	type metadataAlias Metadata

	var (
		fields    map[string]json.RawMessage = make(map[string]json.RawMessage, len(metadata.Raw))
		knownData []byte
		known     map[string]json.RawMessage
	)

	for name, value := range metadata.Raw {
		fields[name] = value
	}

	if knownData, err = json.Marshal(metadataAlias(metadata)); err != nil {
		return
	}

	if err = json.Unmarshal(knownData, &known); err != nil {
		return
	}

	for name, value := range known {
		fields[name] = value
	}

	data, err = json.Marshal(fields)

	return
}

// String returns the event type, clock, and packet identifier.
func (event Event) String() (value string) {
	value = fmt.Sprintf("%08d %9.3fs %-26s (0x%02x)", event.Index, event.Clock, event.Type, event.TypeID)

	return
}

// DecompressedPackets returns a copy of the decrypted, decompressed packet stream.
func (replay *Replay) DecompressedPackets() (packetData []byte) {
	packetData = make([]byte, len(replay.packetData))
	copy(packetData, replay.packetData)

	return
}
