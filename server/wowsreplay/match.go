package wowsreplay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type (
	// Match is a semantic, renderer-oriented reconstruction of a replay.
	Match struct {
		Metadata         Metadata                `json:"metadata"`
		ArenaID          int64                   `json:"arenaId,omitempty"`
		Duration         float32                 `json:"duration"`
		OwnShipID        uint32                  `json:"ownShipId,omitempty"`
		PerspectiveCount int                     `json:"perspectiveCount"`
		PerspectiveTeams []int32                 `json:"perspectiveTeams,omitempty"`
		FogOfWarLimited  bool                    `json:"fogOfWarLimited"`
		Viewport         MatchViewport           `json:"viewport,omitempty"`
		Participants     []MatchParticipant      `json:"participants,omitempty"`
		Entities         map[uint32]*MatchEntity `json:"entities"`
		Traces           []WeaponTrace           `json:"traces,omitempty"`
		Events           []MatchEvent            `json:"events,omitempty"`
		Unknown          MatchUnknownSummary     `json:"unknown"`
	}

	// MatchEntity contains known identity and time-varying state for an entity.
	MatchEntity struct {
		ID          uint32             `json:"id"`
		Type        string             `json:"type"`
		VehicleID   uint32             `json:"vehicleId,omitempty"`
		PlayerID    int64              `json:"playerId,omitempty"`
		PlayerName  string             `json:"playerName,omitempty"`
		ShipID      uint64             `json:"shipId,omitempty"`
		ShipClass   string             `json:"shipClass,omitempty"`
		ShipName    string             `json:"shipName,omitempty"`
		Nation      string             `json:"nation,omitempty"`
		Relation    uint32             `json:"relation,omitempty"`
		TeamID      int32              `json:"teamId,omitempty"`
		TeamKnown   bool               `json:"teamKnown,omitempty"`
		Health      float64            `json:"health,omitempty"`
		MaxHealth   float64            `json:"maxHealth,omitempty"`
		Alive       bool               `json:"alive"`
		Visible     bool               `json:"visible"`
		CreatedAt   float32            `json:"createdAt"`
		DestroyedAt float32            `json:"destroyedAt,omitempty"`
		Positions   []MatchPosition    `json:"positions,omitempty"`
		Changes     []MatchStateChange `json:"changes,omitempty"`
		Objective   *MatchObjective    `json:"objective,omitempty"`
	}

	// MatchViewport identifies the mode-specific playable crop within the terrain map.
	MatchViewport struct {
		BorderParametersID uint64  `json:"borderParametersId,omitempty"`
		Center             Vector3 `json:"center"`
		Width              float32 `json:"width,omitempty"`
		Height             float32 `json:"height,omitempty"`
	}

	// MatchParticipant is one complete metadata-roster entry enriched from GameParams.
	MatchParticipant struct {
		EntityID   uint32 `json:"entityId,omitempty"`
		PlayerID   int64  `json:"playerId"`
		PlayerName string `json:"playerName"`
		ShipID     uint64 `json:"shipId"`
		ShipClass  string `json:"shipClass,omitempty"`
		ShipName   string `json:"shipName,omitempty"`
		Nation     string `json:"nation,omitempty"`
		Relation   uint32 `json:"relation"`
	}

	// MatchObjective contains capture-zone geometry and its latest reconstructed state.
	MatchObjective struct {
		Index       int     `json:"index"`
		Radius      float64 `json:"radius"`
		Type        int     `json:"type"`
		Enabled     bool    `json:"enabled"`
		Visible     bool    `json:"visible"`
		Progress    float64 `json:"progress"`
		HasInvaders bool    `json:"hasInvaders"`
		InvaderTeam int32   `json:"invaderTeam"`
	}

	// MatchPosition is one timestamped entity position and orientation.
	MatchPosition struct {
		Clock    float32  `json:"clock"`
		Position Vector3  `json:"position"`
		Rotation Rotation `json:"rotation"`
	}

	// MatchStateChange records a property value at one point in the match.
	MatchStateChange struct {
		Clock    float32 `json:"clock"`
		Property string  `json:"property"`
		Value    any     `json:"value"`
	}

	// WeaponTrace is a shell, torpedo, aircraft weapon, or explosion trajectory.
	WeaponTrace struct {
		Clock      float32 `json:"clock"`
		Duration   float32 `json:"duration"`
		Kind       string  `json:"kind"`
		OwnerID    int64   `json:"ownerId,omitempty"`
		Start      Vector3 `json:"start"`
		End        Vector3 `json:"end"`
		Velocity   Vector3 `json:"velocity,omitempty"`
		Range      float32 `json:"range,omitempty"`
		Parameters uint32  `json:"parameters,omitempty"`
		ShotID     uint32  `json:"shotId,omitempty"`
		Secondary  bool    `json:"secondary,omitempty"`
	}

	// MatchEvent is a concise battle event suitable for timelines and overlays.
	MatchEvent struct {
		Clock    float32 `json:"clock"`
		Kind     string  `json:"kind"`
		EntityID uint32  `json:"entityId,omitempty"`
		Name     string  `json:"name,omitempty"`
		Data     any     `json:"data,omitempty"`
	}

	// MatchUnknownSummary proves whether any source events were omitted semantically.
	MatchUnknownSummary struct {
		PacketTypes        map[string]int `json:"packetTypes,omitempty"`
		SemanticFailures   int            `json:"semanticFailures"`
		UnresolvedEntities int            `json:"unresolvedEntities"`
	}
)

// BuildMatch reconstructs entities, state changes, combat traces, and notable events.
func (replay *Replay) BuildMatch() (match *Match) {
	match = &Match{
		Metadata: replay.Metadata,
		Entities: make(map[uint32]*MatchEntity),
		Unknown: MatchUnknownSummary{
			PacketTypes: make(map[string]int),
		},
	}

	var participants map[uint64][]Vehicle = make(map[uint64][]Vehicle)

	for _, participant := range replay.Metadata.Vehicles {
		participants[participant.ShipID] = append(participants[participant.ShipID], participant)
		match.Participants = append(match.Participants, MatchParticipant{
			PlayerID: participant.ID, PlayerName: participant.Name, ShipID: participant.ShipID, Relation: participant.Relation,
		})
	}

	for _, event := range replay.Events {
		if event.Clock > match.Duration {
			match.Duration = event.Clock
		}

		switch data := event.Data.(type) {
		case MapData:
			match.ArenaID = data.ArenaID
		case EntityReferenceData:
			if event.Type == "OwnShip" {
				match.OwnShipID = data.EntityID
			}
		case EntityEnterData:
			var entity *MatchEntity = match.ensureEntity(data.EntityID, "", event.Clock)

			entity.applyVisibility(event.Clock, true)
		case EntityCreateData:
			match.ensureEntity(data.EntityID, "", event.Clock).addPosition(event.Clock, data.Position, data.Rotation)
		case CellPlayerCreateData:
			match.ensureEntity(data.EntityID, "Avatar", event.Clock).addPosition(event.Clock, data.Position, data.Rotation)
		case PositionData:
			match.ensureEntity(data.EntityID, "", event.Clock).addPosition(
				event.Clock,
				data.Position,
				data.Rotation,
			)
		case NonVolatilePositionData:
			match.ensureEntity(data.EntityID, "", event.Clock).addPosition(
				event.Clock,
				data.Position,
				data.Rotation,
			)
		case PlayerOrientationData:
			if data.ParentID == 0 {
				match.ensureEntity(data.EntityID, "", event.Clock).addPosition(
					event.Clock,
					data.Position,
					data.Rotation,
				)
			}
		case SemanticEntityCreateData:
			var entity *MatchEntity = match.ensureEntity(data.EntityID, data.EntityTypeName, event.Clock)

			entity.VehicleID = data.VehicleID
			entity.addPosition(event.Clock, data.Position, data.Rotation)
			applyInitialEntityProperties(entity, data.Properties)
			if data.EntityTypeName == "BattleLogic" {
				match.applyInitialBattleProperties(data.Properties)
			}
		case SemanticCellPlayerCreateData:
			var entity *MatchEntity = match.ensureEntity(data.EntityID, data.EntityTypeName, event.Clock)

			entity.VehicleID = data.VehicleID
			entity.addPosition(event.Clock, data.Position, data.Rotation)
			applyInitialEntityProperties(entity, data.Properties)
			if data.DecodeError != "" {
				match.Unknown.SemanticFailures++
			}
		case SemanticPropertyData:
			var entity *MatchEntity = match.ensureEntity(data.EntityID, data.EntityType, event.Clock)

			entity.applyProperty(event.Clock, data.Property, data.Value)
			match.Events = append(match.Events, MatchEvent{
				Clock: event.Clock, Kind: "property", EntityID: data.EntityID, Name: data.Property, Data: data.Value,
			})
			if data.DecodeError != "" {
				match.Unknown.SemanticFailures++
			}
		case SemanticMethodData:
			match.applyMethod(event.Clock, data)
			if data.DecodeError != "" {
				match.Unknown.SemanticFailures++
			}
		case SemanticNestedPropertyData:
			var entity *MatchEntity = match.ensureEntity(data.EntityID, data.EntityType, event.Clock)

			entity.applyNestedProperty(event.Clock, data)
			match.Events = append(match.Events, MatchEvent{
				Clock: event.Clock, Kind: "nested_property", EntityID: data.EntityID, Name: data.Property, Data: data,
			})
			if data.DecodeError != "" {
				match.Unknown.SemanticFailures++
				match.Unknown.PacketTypes[event.Type]++
			}
		default:
			if len(event.Type) >= 8 && event.Type[:8] == "Unknown_" {
				match.Unknown.PacketTypes[event.Type]++
			}
		}

		if event.Type != "NestedPropertyUpdate" &&
			event.Type != "StreamEnd" &&
			event.Data == nil &&
			len(event.RawPayload) > 0 &&
			!(len(event.Type) >= 8 && event.Type[:8] == "Unknown_") {
			match.Unknown.PacketTypes[event.Type]++
		}

		if event.Type == "EntityLeave" {
			if reference, ok := event.Data.(EntityReferenceData); ok {
				var entity *MatchEntity = match.ensureEntity(reference.EntityID, "", event.Clock)

				entity.applyVisibility(event.Clock, false)
				match.Events = append(match.Events, MatchEvent{
					Clock: event.Clock, Kind: "leave", EntityID: reference.EntityID,
				})
			}
		}
	}

	for _, entity := range match.Entities {
		if entity.Type == "" {
			match.Unknown.UnresolvedEntities++
		}
	}

	assignParticipants(match, participants)

	if ownShip, found := match.Entities[match.OwnShipID]; found && ownShip.TeamKnown {
		match.PerspectiveTeams = append(match.PerspectiveTeams, ownShip.TeamID)
	}

	sort.SliceStable(match.Traces, func(left, right int) bool {
		return match.Traces[left].Clock < match.Traces[right].Clock
	})
	match.PerspectiveCount = 1
	match.FogOfWarLimited = true

	return
}

// MergeMatches combines compatible replay perspectives to reduce fog-of-war gaps.
func MergeMatches(primary *Match, additional ...*Match) (merged *Match, err error) {
	if primary == nil {
		err = errors.New("primary match is nil")

		return
	}

	merged = cloneMatch(primary)
	for _, perspective := range additional {
		if perspective == nil {
			continue
		}

		if err = validateMatchIdentity(merged, perspective); err != nil {
			merged = nil

			return
		}

		for entityID, source := range perspective.Entities {
			var destination *MatchEntity = merged.ensureEntity(entityID, source.Type, source.CreatedAt)

			mergeEntity(destination, source)
		}

		mergeTraces(merged, perspective.Traces)
		merged.Events = append(merged.Events, perspective.Events...)
		merged.Duration = max(merged.Duration, perspective.Duration)
		merged.PerspectiveCount += max(perspective.PerspectiveCount, 1)

		for packetType, count := range perspective.Unknown.PacketTypes {
			merged.Unknown.PacketTypes[packetType] += count
		}
		merged.Unknown.SemanticFailures += perspective.Unknown.SemanticFailures
		for _, teamID := range perspective.PerspectiveTeams {
			if !containsTeam(merged.PerspectiveTeams, teamID) {
				merged.PerspectiveTeams = append(merged.PerspectiveTeams, teamID)
			}
		}
	}

	for _, entity := range merged.Entities {
		sort.SliceStable(entity.Positions, func(left, right int) bool {
			return entity.Positions[left].Clock < entity.Positions[right].Clock
		})
		sort.SliceStable(entity.Changes, func(left, right int) bool {
			return entity.Changes[left].Clock < entity.Changes[right].Clock
		})
	}
	sort.SliceStable(merged.Traces, func(left, right int) bool {
		return merged.Traces[left].Clock < merged.Traces[right].Clock
	})
	sort.SliceStable(merged.Events, func(left, right int) bool {
		return merged.Events[left].Clock < merged.Events[right].Clock
	})

	merged.FogOfWarLimited = len(merged.PerspectiveTeams) < int(max(merged.Metadata.TeamsCount, 2))

	return
}

// cloneMatch copies the mutable match collections used by perspective merging.
func cloneMatch(source *Match) (clone *Match) {
	clone = &Match{
		Metadata:         source.Metadata,
		ArenaID:          source.ArenaID,
		Duration:         source.Duration,
		OwnShipID:        source.OwnShipID,
		PerspectiveCount: max(source.PerspectiveCount, 1),
		PerspectiveTeams: append([]int32(nil), source.PerspectiveTeams...),
		FogOfWarLimited:  source.FogOfWarLimited,
		Viewport:         source.Viewport,
		Participants:     append([]MatchParticipant(nil), source.Participants...),
		Entities:         make(map[uint32]*MatchEntity, len(source.Entities)),
		Traces:           append([]WeaponTrace(nil), source.Traces...),
		Events:           append([]MatchEvent(nil), source.Events...),
		Unknown: MatchUnknownSummary{
			PacketTypes:        make(map[string]int, len(source.Unknown.PacketTypes)),
			SemanticFailures:   source.Unknown.SemanticFailures,
			UnresolvedEntities: source.Unknown.UnresolvedEntities,
		},
	}

	for packetType, count := range source.Unknown.PacketTypes {
		clone.Unknown.PacketTypes[packetType] = count
	}

	for entityID, entity := range source.Entities {
		var copied MatchEntity = *entity

		copied.Positions = append([]MatchPosition(nil), entity.Positions...)
		copied.Changes = append([]MatchStateChange(nil), entity.Changes...)
		if entity.Objective != nil {
			var objective MatchObjective = *entity.Objective

			copied.Objective = &objective
		}
		clone.Entities[entityID] = &copied
	}

	return
}

// containsTeam checks whether a perspective already covers one team.
func containsTeam(teams []int32, teamID int32) (found bool) {
	for _, existing := range teams {
		if existing == teamID {
			found = true

			return
		}
	}

	return
}

// validateMatchIdentity prevents accidental merging of different battles.
func validateMatchIdentity(primary, perspective *Match) (err error) {
	if primary.ArenaID != 0 && perspective.ArenaID != 0 && primary.ArenaID != perspective.ArenaID {
		err = fmt.Errorf("arena %d does not match arena %d", perspective.ArenaID, primary.ArenaID)
	} else if primary.Metadata.MapName != perspective.Metadata.MapName ||
		primary.Metadata.DateTime != perspective.Metadata.DateTime {
		err = errors.New("match map/date identity does not match")
	}

	return
}

// mergeEntity unions observations of one shared BigWorld entity.
func mergeEntity(destination, source *MatchEntity) {
	if destination.Type == "" {
		destination.Type = source.Type
	}
	if destination.VehicleID == 0 {
		destination.VehicleID = source.VehicleID
	}
	if destination.PlayerID == 0 {
		destination.PlayerID = source.PlayerID
		destination.PlayerName = source.PlayerName
		destination.ShipID = source.ShipID
		destination.Relation = source.Relation
	}
	if destination.ShipClass == "" {
		destination.ShipClass = source.ShipClass
	}
	if destination.ShipName == "" {
		destination.ShipName = source.ShipName
	}
	if destination.Nation == "" {
		destination.Nation = source.Nation
	}
	if !destination.TeamKnown && source.TeamKnown {
		destination.TeamKnown = true
		destination.TeamID = source.TeamID
	}
	destination.MaxHealth = max(destination.MaxHealth, source.MaxHealth)
	destination.CreatedAt = min(destination.CreatedAt, source.CreatedAt)
	destination.DestroyedAt = max(destination.DestroyedAt, source.DestroyedAt)
	destination.Positions = append(destination.Positions, source.Positions...)
	destination.Changes = append(destination.Changes, source.Changes...)
	if destination.Objective == nil && source.Objective != nil {
		var objective MatchObjective = *source.Objective

		destination.Objective = &objective
	}
}

// mergeTraces adds observations not already present in another perspective.
func mergeTraces(match *Match, traces []WeaponTrace) {
	var seen map[WeaponTrace]bool = make(map[WeaponTrace]bool, len(match.Traces))

	for _, trace := range match.Traces {
		seen[trace] = true
	}

	for _, trace := range traces {
		if !seen[trace] {
			match.Traces = append(match.Traces, trace)
			seen[trace] = true
		}
	}
}

// ensureEntity returns an existing entity or creates a conservative placeholder.
func (match *Match) ensureEntity(entityID uint32, entityType string, clock float32) (entity *MatchEntity) {
	var found bool

	if entity, found = match.Entities[entityID]; !found {
		entity = &MatchEntity{
			ID:        entityID,
			Type:      entityType,
			Alive:     true,
			Visible:   true,
			CreatedAt: clock,
		}
		match.Entities[entityID] = entity
	} else if entity.Type == "" && entityType != "" {
		entity.Type = entityType
	}

	return
}

// addPosition appends a valid position while suppressing exact duplicates.
func (entity *MatchEntity) addPosition(clock float32, position Vector3, rotation Rotation) {
	if !validMapPosition(position) {
		return
	}

	if len(entity.Positions) > 0 {
		var previous MatchPosition = entity.Positions[len(entity.Positions)-1]

		if previous.Clock == clock && previous.Position == position {
			return
		}
	}

	entity.Positions = append(entity.Positions, MatchPosition{
		Clock: clock, Position: position, Rotation: rotation,
	})
}

// applyProperty updates canonical entity fields while preserving the original value.
func (entity *MatchEntity) applyProperty(clock float32, property string, value any) {
	entity.Changes = append(entity.Changes, MatchStateChange{
		Clock: clock, Property: property, Value: value,
	})

	switch property {
	case "health":
		entity.Health = numericValue(value)
	case "maxHealth":
		entity.MaxHealth = numericValue(value)
	case "teamId":
		entity.TeamID = int32(numericValue(value))
		entity.TeamKnown = true
	case "isAlive":
		entity.Alive = numericValue(value) != 0
		if !entity.Alive {
			entity.DestroyedAt = clock
		}
	case "isInvisible":
		entity.Visible = numericValue(value) == 0
	}
}

// applyInitialEntityProperties initializes identity, health, team, and visibility.
func applyInitialEntityProperties(
	entity *MatchEntity,
	properties map[string]any,
) {
	for name, value := range properties {
		entity.applyProperty(entity.CreatedAt, name, value)
	}

	if shipConfig, ok := properties["shipConfig"].([]byte); ok && len(shipConfig) >= 8 {
		entity.ShipID = uint64(binary.LittleEndian.Uint32(shipConfig[4:8]))
	}

	if entity.Type == "InteractiveZone" {
		entity.initializeObjective(properties)
	}
}

// applyInitialBattleProperties captures the mode-specific map-border crop.
func (match *Match) applyInitialBattleProperties(properties map[string]any) {
	var border map[string]any = anyMap(properties["mapBorder"])

	match.Viewport.BorderParametersID = uint64(numericValue(border["paramsId"]))
	match.Viewport.Center = vectorValue(border["position"])
}

// applyNestedProperty advances canonical objective state from a decoded nested update.
func (entity *MatchEntity) applyNestedProperty(clock float32, update SemanticNestedPropertyData) {
	if entity.Objective == nil ||
		update.Property != "componentsState" ||
		len(update.Path) != 1 ||
		fmt.Sprint(update.Path[0]) != "captureLogic" ||
		update.Action != "set_key" {
		return
	}

	entity.applyObjectiveProperty(clock, update.Key, update.Value)
}

// initializeObjective extracts capture-zone geometry and initial control state.
func (entity *MatchEntity) initializeObjective(properties map[string]any) {
	var (
		components   map[string]any = anyMap(properties["componentsState"])
		captureLogic map[string]any = anyMap(components["captureLogic"])
		controlPoint map[string]any = anyMap(components["controlPoint"])
	)

	entity.Objective = &MatchObjective{
		Index:       int(numericValue(controlPoint["index"])),
		Radius:      numericValue(properties["radius"]),
		Type:        int(numericValue(controlPoint["type"])),
		Enabled:     numericValue(captureLogic["isEnabled"]) != 0,
		Visible:     numericValue(captureLogic["isVisible"]) != 0,
		Progress:    numericValue(captureLogic["progress"]),
		HasInvaders: numericValue(captureLogic["hasInvaders"]) != 0,
		InvaderTeam: int32(numericValue(captureLogic["invaderTeam"])),
	}

	for name, value := range map[string]any{
		"radius":      entity.Objective.Radius,
		"index":       entity.Objective.Index,
		"type":        entity.Objective.Type,
		"isEnabled":   entity.Objective.Enabled,
		"isVisible":   entity.Objective.Visible,
		"progress":    entity.Objective.Progress,
		"hasInvaders": entity.Objective.HasInvaders,
		"invaderTeam": entity.Objective.InvaderTeam,
	} {
		entity.applyObjectiveProperty(entity.CreatedAt, name, value)
	}
}

// applyObjectiveProperty updates one canonical capture-zone field and its timeline.
func (entity *MatchEntity) applyObjectiveProperty(clock float32, property string, value any) {
	if entity.Objective == nil {
		return
	}

	entity.Changes = append(entity.Changes, MatchStateChange{
		Clock: clock, Property: "__objective." + property, Value: value,
	})

	switch property {
	case "radius":
		entity.Objective.Radius = numericValue(value)
	case "index":
		entity.Objective.Index = int(numericValue(value))
	case "type":
		entity.Objective.Type = int(numericValue(value))
	case "isEnabled":
		entity.Objective.Enabled = booleanValue(value)
	case "isVisible":
		entity.Objective.Visible = booleanValue(value)
	case "progress":
		entity.Objective.Progress = numericValue(value)
	case "hasInvaders":
		entity.Objective.HasInvaders = booleanValue(value)
	case "invaderTeam":
		entity.Objective.InvaderTeam = int32(numericValue(value))
	}
}

// EnrichShipParameters attaches the game-defined class to reconstructed ships.
func (match *Match) EnrichShipParameters(ships map[uint64]ShipParameters) {
	for _, entity := range match.Entities {
		var (
			found bool
			ship  ShipParameters
		)

		if ship, found = ships[entity.ShipID]; found {
			entity.ShipClass = ship.Species
			entity.ShipName = displayShipName(ship)
			entity.Nation = ship.Nation
		}
	}

	for index := range match.Participants {
		var (
			found bool
			ship  ShipParameters
		)

		if ship, found = ships[match.Participants[index].ShipID]; found {
			match.Participants[index].ShipClass = ship.Species
			match.Participants[index].ShipName = displayShipName(ship)
			match.Participants[index].Nation = ship.Nation
		}
	}
}

// EnrichMapBorders resolves the captured viewport parameter into world dimensions.
func (match *Match) EnrichMapBorders(borders map[uint64]MapBorderParameters) {
	var (
		border MapBorderParameters
		found  bool
	)

	if border, found = borders[match.Viewport.BorderParametersID]; found {
		match.Viewport.Width = border.Width
		match.Viewport.Height = border.Height
	}
}

// EnrichProjectileParameters applies real torpedo ranges to transmitted launch vectors.
func (match *Match) EnrichProjectileParameters(projectiles map[uint64]ProjectileParameters) {
	for index := range match.Traces {
		if match.Traces[index].Kind != "torpedo" {
			continue
		}

		var (
			distance   float32 = 240
			found      bool
			parameters ProjectileParameters
			speed      float32 = float32(math.Hypot(
				float64(match.Traces[index].Velocity.X),
				float64(match.Traces[index].Velocity.Z),
			))
		)

		if parameters, found = projectiles[uint64(match.Traces[index].Parameters)]; found &&
			parameters.MaxDistance > 0 {
			distance = parameters.MaxDistance
		}
		if speed <= 0 {
			continue
		}

		match.Traces[index].Range = distance
		match.Traces[index].Duration = distance / speed
		match.Traces[index].End = Vector3{
			X: match.Traces[index].Start.X + match.Traces[index].Velocity.X*match.Traces[index].Duration,
			Y: match.Traces[index].Start.Y + match.Traces[index].Velocity.Y*match.Traces[index].Duration,
			Z: match.Traces[index].Start.Z + match.Traces[index].Velocity.Z*match.Traces[index].Duration,
		}
	}
}

// displayShipName converts the stable GameParams name into a compact roster label.
func displayShipName(ship ShipParameters) (name string) {
	name = ship.Name
	name = strings.TrimPrefix(name, ship.Index+"_")
	name = strings.ReplaceAll(name, "_", " ")
	if name == "" {
		name = ship.Index
	}

	return
}

// assignParticipants correlates ship parameters with the team-aware metadata roster.
func assignParticipants(match *Match, participants map[uint64][]Vehicle) {
	var (
		ownTeam      int32
		ownTeamKnown bool
	)

	if ownShip, found := match.Entities[match.OwnShipID]; found && ownShip.TeamKnown {
		ownTeam = ownShip.TeamID
		ownTeamKnown = true
	}

	for _, entity := range match.Entities {
		if entity.Type != "Vehicle" || entity.ShipID == 0 {
			continue
		}

		var desiredRelation uint32

		if entity.ID == match.OwnShipID {
			desiredRelation = 0
		} else if ownTeamKnown && entity.TeamKnown && entity.TeamID == ownTeam {
			desiredRelation = 1
		} else if ownTeamKnown && entity.TeamKnown {
			desiredRelation = 2
		}

		var candidates []Vehicle = participants[entity.ShipID]

		for index, participant := range candidates {
			if participant.Relation != desiredRelation {
				continue
			}

			entity.PlayerID = participant.ID
			entity.PlayerName = participant.Name
			entity.Relation = participant.Relation
			participants[entity.ShipID] = append(candidates[:index], candidates[index+1:]...)

			for rosterIndex := range match.Participants {
				if match.Participants[rosterIndex].PlayerID == participant.ID {
					match.Participants[rosterIndex].EntityID = entity.ID

					break
				}
			}

			break
		}
	}
}

// applyMethod turns named combat methods into traces and timeline entries.
func (match *Match) applyMethod(clock float32, method SemanticMethodData) {
	match.Events = append(match.Events, MatchEvent{
		Clock: clock, Kind: "method", EntityID: method.EntityID, Name: method.Method, Data: method.Arguments,
	})

	switch method.Method {
	case "receiveVehicleDeath":
		if len(method.Arguments) >= 2 {
			var victimID uint32 = uint32(numericValue(method.Arguments[0].Value))
			var victim *MatchEntity = match.ensureEntity(victimID, "Vehicle", clock)

			victim.Alive = false
			victim.DestroyedAt = clock
			victim.Changes = append(victim.Changes, MatchStateChange{
				Clock: clock, Property: "__alive", Value: false,
			})
			match.Events = append(match.Events, MatchEvent{
				Clock:    clock,
				Kind:     "death",
				EntityID: victimID,
				Data:     map[string]any{"killerId": method.Arguments[1].Value},
			})
		}
	case "updateMinimapVisionInfo":
		match.applyMinimapUpdates(clock, method)
	case "receiveArtilleryShots":
		match.appendPackedTraces(clock, method, "shell", "shots", "pos", "tarPos", "serverTimeLeft")
	case "receiveTorpedoes":
		match.appendTorpedoTraces(clock, method)
	case "receivePlaneProjectilePack", "receivePlaneSkipBombPacks", "receivePlaneRocketPacks":
		match.appendPlaneTraces(clock, method)
	case "receiveDepthChargesPacks":
		match.appendPackedTraces(clock, method, "depth_charge", "depthCharges", "pos", "pos", "serverTimeLeft")
	case "receiveExplosions":
		match.appendExplosionTraces(clock, method)
	}
}

// applyMinimapUpdates reconstructs visible and last-known ship placements.
func (match *Match) applyMinimapUpdates(clock float32, method SemanticMethodData) {
	if len(method.Arguments) == 0 {
		return
	}

	for _, updateValue := range anySlice(method.Arguments[0].Value) {
		var (
			update     map[string]any = anyMap(updateValue)
			entityID   uint32         = uint32(numericValue(update["vehicleID"]))
			packedData uint32         = uint32(numericValue(update["packedData"]))
			rawX       uint32         = packedData & 0x7ff
			rawZ       uint32         = (packedData >> 11) & 0x7ff
			heading    uint32         = (packedData >> 22) & 0xff
			disappears bool           = packedData&(1<<31) != 0
			entity     *MatchEntity   = match.ensureEntity(entityID, "Vehicle", clock)
		)

		if rawX == 0 && rawZ == 0 {
			entity.applyVisibility(clock, false)

			continue
		}

		if !entity.hasRecentPosition(clock, 0.5) {
			entity.addPosition(clock, Vector3{
				X: float32(rawX)/2047*5000 - 2500,
				Z: float32(rawZ)/2047*5000 - 2500,
			}, Rotation{
				Yaw: (float32(heading)/256*360 - 180) * math.Pi / 180,
			})
		}
		entity.applyVisibility(clock, !disappears)
	}
}

// hasRecentPosition prevents coarse minimap samples from replacing fresh world positions.
func (entity *MatchEntity) hasRecentPosition(clock, tolerance float32) (recent bool) {
	if len(entity.Positions) > 0 {
		recent = clock-entity.Positions[len(entity.Positions)-1].Clock <= tolerance
	}

	return
}

// applyVisibility records visibility only when the state actually changes.
func (entity *MatchEntity) applyVisibility(clock float32, visible bool) {
	if entity.Visible == visible {
		return
	}

	entity.Visible = visible
	entity.Changes = append(entity.Changes, MatchStateChange{
		Clock: clock, Property: "__visible", Value: visible,
	})
}

// appendPackedTraces handles artillery and depth-charge packet arrays.
func (match *Match) appendPackedTraces(
	clock float32,
	method SemanticMethodData,
	kind,
	itemsName,
	startName,
	endName,
	durationName string,
) {
	if len(method.Arguments) == 0 {
		return
	}

	for _, packValue := range anySlice(method.Arguments[0].Value) {
		var pack map[string]any = anyMap(packValue)

		for _, itemValue := range anySlice(pack[itemsName]) {
			var item map[string]any = anyMap(itemValue)
			var duration float32 = float32(numericValue(item[durationName]))

			if duration <= 0 {
				duration = 2
			}

			match.Traces = append(match.Traces, WeaponTrace{
				Clock:      clock,
				Duration:   duration,
				Kind:       kind,
				OwnerID:    int64(numericValue(pack["ownerID"])),
				Start:      vectorValue(item[startName]),
				End:        vectorValue(item[endName]),
				Parameters: uint32(numericValue(pack["paramsID"])),
				ShotID:     uint32(numericValue(item["shotID"])),
				Secondary:  uint32(numericValue(item["gunBarrelID"])) >= 32768,
			})
		}
	}
}

// appendTorpedoTraces starts range-limited tracks when torpedoes become observable.
func (match *Match) appendTorpedoTraces(clock float32, method SemanticMethodData) {
	if len(method.Arguments) == 0 {
		return
	}

	for _, packValue := range anySlice(method.Arguments[0].Value) {
		var (
			pack       map[string]any = anyMap(packValue)
			ownerID    int64          = int64(numericValue(pack["ownerID"]))
			parameters uint32         = uint32(numericValue(pack["paramsID"]))
		)

		for _, itemValue := range anySlice(pack["torpedoes"]) {
			var (
				distance float32 = 240
				duration float32
				item     map[string]any = anyMap(itemValue)
				speed    float32
				start    Vector3 = vectorValue(item["pos"])
				velocity Vector3 = vectorValue(item["dir"])
			)

			speed = float32(math.Hypot(float64(velocity.X), float64(velocity.Z)))
			if speed <= 0 {
				continue
			}
			duration = distance / speed
			match.Traces = append(match.Traces, WeaponTrace{
				Clock:    clock,
				Duration: duration,
				Kind:     "torpedo",
				OwnerID:  ownerID,
				Start:    start,
				End: Vector3{
					X: start.X + velocity.X*duration,
					Y: start.Y + velocity.Y*duration,
					Z: start.Z + velocity.Z*duration,
				},
				Velocity:   velocity,
				Range:      distance,
				Parameters: parameters,
				ShotID:     uint32(numericValue(item["shotID"])),
			})
		}
	}
}

// appendPlaneTraces maps transmitted aircraft impact points into concise attack traces.
func (match *Match) appendPlaneTraces(clock float32, method SemanticMethodData) {
	if len(method.Arguments) == 0 {
		return
	}

	for _, packValue := range anySlice(method.Arguments[0].Value) {
		var (
			pack     map[string]any = anyMap(packValue)
			start    Vector3        = vectorValue(pack["squadronToTarget"])
			duration float32        = float32(numericValue(pack["timeLeft"]))
		)

		if duration <= 0 {
			duration = float32(numericValue(pack["fallTime"]))
		}

		for _, itemValue := range anySlice(pack["projectiles"]) {
			match.Traces = append(match.Traces, WeaponTrace{
				Clock:      clock,
				Duration:   duration,
				Kind:       "aircraft",
				OwnerID:    int64(numericValue(pack["squadronId"])),
				Start:      start,
				End:        vectorValue(anyMap(itemValue)["impactPoint"]),
				Parameters: uint32(numericValue(pack["bombParamsId"])),
			})
		}
	}
}

// appendExplosionTraces preserves impact positions as short-lived point traces.
func (match *Match) appendExplosionTraces(clock float32, method SemanticMethodData) {
	if len(method.Arguments) == 0 {
		return
	}

	for _, itemValue := range anySlice(method.Arguments[0].Value) {
		var item map[string]any = anyMap(itemValue)
		var position Vector3 = vectorValue(item["pos"])

		match.Traces = append(match.Traces, WeaponTrace{
			Clock:      clock,
			Duration:   0.8,
			Kind:       "explosion",
			Start:      position,
			End:        position,
			Parameters: uint32(numericValue(item["paramsID"])),
		})
	}
}

// numericValue converts any decoded RPC number to float64.
func numericValue(value any) (number float64) {
	switch typed := value.(type) {
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case float32:
		number = float64(typed)
	case float64:
		number = typed
	}

	return
}

// booleanValue accepts both decoded booleans and numeric protocol flags.
func booleanValue(value any) (result bool) {
	var (
		boolean bool
		found   bool
	)

	if boolean, found = value.(bool); found {
		result = boolean
	} else {
		result = numericValue(value) != 0
	}

	return
}

// vectorValue converts a decoded VECTOR3 or compatible map into Vector3.
func vectorValue(value any) (vector Vector3) {
	switch typed := value.(type) {
	case Vector3:
		vector = typed
	case map[string]any:
		vector.X = float32(numericValue(typed["x"]))
		vector.Y = float32(numericValue(typed["y"]))
		vector.Z = float32(numericValue(typed["z"]))
	}

	return
}

// anySlice safely unwraps a decoded RPC array.
func anySlice(value any) (values []any) {
	values, _ = value.([]any)

	return
}

// anyMap safely unwraps a decoded fixed dictionary.
func anyMap(value any) (values map[string]any) {
	values, _ = value.(map[string]any)

	return
}

// String summarizes the reconstructed match.
func (match *Match) String() (summary string) {
	summary = fmt.Sprintf(
		"%s on %s: %d entities, %d traces, %.1fs",
		match.Metadata.GameType,
		match.Metadata.MapDisplayName,
		len(match.Entities),
		len(match.Traces),
		match.Duration,
	)

	return
}
