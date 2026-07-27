package wowsreplay

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type (
	// SemanticDecoder decodes version-matched entity methods and properties.
	SemanticDecoder struct {
		schema   replaySchema
		entities map[uint32]*semanticEntityState
		version  string
		build    uint32
		checked  bool
	}

	semanticEntityState struct {
		entityType uint16
		properties []any
	}

	// SemanticArgument is one named or positional RPC method argument.
	SemanticArgument struct {
		Name  string `json:"name,omitempty"`
		Value any    `json:"value"`
	}

	// SemanticMethodData is a versioned entity method with decoded arguments.
	SemanticMethodData struct {
		EntityID    uint32             `json:"entityId"`
		EntityType  string             `json:"entityType,omitempty"`
		MethodID    uint32             `json:"methodId"`
		Method      string             `json:"method,omitempty"`
		Arguments   []SemanticArgument `json:"arguments,omitempty"`
		Leftover    []byte             `json:"leftover,omitempty"`
		DecodeError string             `json:"decodeError,omitempty"`
	}

	// SemanticPropertyData is a versioned entity property update.
	SemanticPropertyData struct {
		EntityID    uint32 `json:"entityId"`
		EntityType  string `json:"entityType,omitempty"`
		PropertyID  uint32 `json:"propertyId"`
		Property    string `json:"property,omitempty"`
		Value       any    `json:"value,omitempty"`
		Leftover    []byte `json:"leftover,omitempty"`
		DecodeError string `json:"decodeError,omitempty"`
	}

	// SemanticNestedPropertyData describes an update within an array or fixed dictionary.
	SemanticNestedPropertyData struct {
		EntityID    uint32 `json:"entityId"`
		EntityType  string `json:"entityType,omitempty"`
		PropertyID  uint32 `json:"propertyId"`
		Property    string `json:"property,omitempty"`
		Path        []any  `json:"path,omitempty"`
		Action      string `json:"action,omitempty"`
		Key         string `json:"key,omitempty"`
		Index       int    `json:"index,omitempty"`
		Start       int    `json:"start,omitempty"`
		Stop        int    `json:"stop,omitempty"`
		Value       any    `json:"value,omitempty"`
		Values      []any  `json:"values,omitempty"`
		DecodeError string `json:"decodeError,omitempty"`
	}

	// SemanticEntityCreateData extends an entity creation with its type and initial properties.
	SemanticEntityCreateData struct {
		EntityCreateData
		EntityTypeName string         `json:"entityTypeName,omitempty"`
		Properties     map[string]any `json:"properties,omitempty"`
		Leftover       []byte         `json:"leftover,omitempty"`
		DecodeError    string         `json:"decodeError,omitempty"`
	}

	// SemanticPlayerCreateData describes a base-player creation packet.
	SemanticPlayerCreateData struct {
		EntityID       uint32         `json:"entityId"`
		EntityType     uint16         `json:"entityType"`
		EntityTypeName string         `json:"entityTypeName,omitempty"`
		Properties     map[string]any `json:"properties,omitempty"`
		ComponentData  []byte         `json:"componentData,omitempty"`
		DecodeError    string         `json:"decodeError,omitempty"`
	}

	// SemanticCellPlayerCreateData extends a cell-player creation with decoded properties.
	SemanticCellPlayerCreateData struct {
		CellPlayerCreateData
		EntityTypeName string         `json:"entityTypeName,omitempty"`
		Properties     map[string]any `json:"properties,omitempty"`
		ComponentData  []byte         `json:"componentData,omitempty"`
		DecodeError    string         `json:"decodeError,omitempty"`
	}
)

// NewSemanticDecoder constructs a stateful decoder from an imported resource bundle.
func NewSemanticDecoder(bundle *ResourceBundle) (decoder *SemanticDecoder, err error) {
	decoder = &SemanticDecoder{
		entities: make(map[uint32]*semanticEntityState),
		version:  bundle.Manifest.Version,
		build:    bundle.Manifest.Build,
	}
	if decoder.schema, err = loadReplaySchema(bundle); err != nil {
		decoder = nil
	}

	return
}

// DecodeEvent resolves entity types, properties, methods, and their recursive arguments.
func (decoder *SemanticDecoder) DecodeEvent(
	metadata Metadata,
	event Event,
) (data any, handled bool, err error) {
	if !decoder.checked {
		decoder.checked = true
		if err = decoder.checkVersion(metadata.ClientVersionFromExe); err != nil {
			return
		}
	}

	switch event.Type {
	case "BasePlayerCreate":
		data = decoder.decodeBasePlayerCreate(event.RawPayload, false)
		handled = true
	case "BasePlayerCreateStub":
		data = decoder.decodeBasePlayerCreate(event.RawPayload, true)
		handled = true
	case "CellPlayerCreate":
		data = decoder.decodeCellPlayerCreate(event.RawPayload)
		handled = true
	case "EntityCreate":
		data = decoder.decodeEntityCreate(event.RawPayload)
		handled = true
	case "EntityMethod":
		data = decoder.decodeEntityMethod(event.RawPayload)
		handled = true
	case "EntityProperty":
		data = decoder.decodeEntityProperty(event.RawPayload)
		handled = true
	case "NestedPropertyUpdate":
		data = decoder.decodeNestedProperty(event.RawPayload)
		handled = true
	}

	return
}

// ParseFileWithResources parses a replay using definitions from an exact resource bundle.
func ParseFileWithResources(path string, bundle *ResourceBundle) (replay *Replay, err error) {
	var (
		decoder *SemanticDecoder
		file    *os.File
		parser  *Parser = NewParser()
	)

	if decoder, err = NewSemanticDecoder(bundle); err != nil {
		return
	}
	parser.EventDecoders = append(parser.EventDecoders, decoder)

	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	replay, err = parser.Parse(file)

	return
}

// DecodeWithResources applies versioned semantics to an already parsed replay in place.
func (replay *Replay) DecodeWithResources(bundle *ResourceBundle) (err error) {
	var decoder *SemanticDecoder

	if decoder, err = NewSemanticDecoder(bundle); err != nil {
		return
	}

	for index := range replay.Events {
		var (
			data    any
			handled bool
		)

		if data, handled, err = decoder.DecodeEvent(replay.Metadata, replay.Events[index]); err != nil {
			err = fmt.Errorf("decode event %d: %w", index, err)

			return
		}

		if handled {
			replay.Events[index].Data = data
		}
	}

	return
}

// checkVersion prevents silent decoding with schema data from another game build.
func (decoder *SemanticDecoder) checkVersion(clientVersion string) (err error) {
	var components []string = strings.Split(clientVersion, ",")

	if len(components) != 4 {
		err = fmt.Errorf("replay client version %q is malformed", clientVersion)

		return
	}

	var (
		build   uint64
		version string = strings.Join(components[:3], ".")
	)

	if build, err = strconv.ParseUint(strings.TrimSpace(components[3]), 10, 32); err != nil {
		return
	}

	if strings.ReplaceAll(version, " ", "") != decoder.version || uint32(build) != decoder.build {
		err = fmt.Errorf(
			"resource bundle %s build %d does not match replay %s",
			decoder.version,
			decoder.build,
			clientVersion,
		)
	}

	return
}

// decodeBasePlayerCreate registers a player entity and reads its base properties.
func (decoder *SemanticDecoder) decodeBasePlayerCreate(payload []byte, stub bool) (data SemanticPlayerCreateData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	data.EntityType = reader.uint16()
	decoder.registerEntity(data.EntityID, data.EntityType)

	var (
		entity *entitySchema
		err    error
	)

	if entity, err = decoder.entity(data.EntityType); err != nil {
		data.DecodeError = err.Error()

		return
	}
	data.EntityTypeName = entity.name

	if stub {
		data.ComponentData = reader.remaining()

		return
	}

	if data.Properties, err = decodeProperties(&reader, entity.baseProperties); err != nil {
		data.DecodeError = err.Error()
	}
	data.ComponentData = reader.remaining()

	return
}

// decodeCellPlayerCreate decodes the complete client-visible player state.
func (decoder *SemanticDecoder) decodeCellPlayerCreate(payload []byte) (data SemanticCellPlayerCreateData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	data.SpaceID = reader.uint32()
	data.VehicleID = reader.uint32()
	data.Position = reader.vector3()
	data.Rotation = reader.rotation()
	data.StateSize = reader.uint32()

	var (
		entity      *entitySchema
		entityState *semanticEntityState
		err         error
		found       bool
	)

	if entityState, found = decoder.entities[data.EntityID]; !found {
		data.DecodeError = fmt.Sprintf("unknown entity %d", data.EntityID)

		return
	}

	if entity, err = decoder.entity(entityState.entityType); err != nil {
		data.DecodeError = err.Error()

		return
	}
	data.EntityTypeName = entity.name

	var stateReader payloadReader

	if stateReader.data, err = reader.take(int(data.StateSize)); err != nil {
		data.DecodeError = err.Error()

		return
	}

	if data.Properties, err = decodeProperties(&stateReader, entity.internalProperties); err != nil {
		data.DecodeError = err.Error()
	}
	data.ComponentData = append(stateReader.remaining(), reader.remaining()...)

	return
}

// decodeEntityCreate registers an entity and decodes its sparse initial property set.
func (decoder *SemanticDecoder) decodeEntityCreate(payload []byte) (data SemanticEntityCreateData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	data.EntityType = reader.uint16()
	data.VehicleID = reader.uint32()
	data.SpaceID = reader.uint32()
	data.Position = reader.vector3()
	data.Rotation = reader.rotation()
	data.StateSize = reader.uint32()
	decoder.registerEntity(data.EntityID, data.EntityType)

	var (
		entity *entitySchema
		err    error
	)

	if entity, err = decoder.entity(data.EntityType); err != nil {
		data.DecodeError = err.Error()

		return
	}
	data.EntityTypeName = entity.name

	var state payloadReader

	state.data = reader.remaining()
	if len(state.data) == 0 {
		return
	}

	var count int = int(state.uint8())

	data.Properties = make(map[string]any, count)
	for index := 0; index < count; index++ {
		var propertyID int = int(state.uint8())

		if propertyID >= len(entity.properties) {
			data.DecodeError = fmt.Sprintf("property %d exceeds %s schema", propertyID, entity.name)

			break
		}

		var value any

		if value, err = entity.properties[propertyID].kind.parseValue(&state); err != nil {
			data.DecodeError = fmt.Sprintf("%s: %v", entity.properties[propertyID].name, err)

			break
		}
		data.Properties[entity.properties[propertyID].name] = value
		decoder.entities[data.EntityID].properties[propertyID] = value
	}
	data.Leftover = state.remaining()

	return
}

// decodeEntityMethod resolves and decodes one exposed client method.
func (decoder *SemanticDecoder) decodeEntityMethod(payload []byte) (data SemanticMethodData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	data.MethodID = reader.uint32()
	var payloadSize uint32 = reader.uint32()

	var (
		entity *entitySchema
		state  *semanticEntityState
		err    error
		found  bool
	)

	if state, found = decoder.entities[data.EntityID]; !found {
		data.DecodeError = fmt.Sprintf("unknown entity %d", data.EntityID)

		return
	}

	if entity, err = decoder.entity(state.entityType); err != nil {
		data.DecodeError = err.Error()

		return
	}
	data.EntityType = entity.name

	if int(data.MethodID) >= len(entity.clientMethods) {
		data.DecodeError = fmt.Sprintf("method %d exceeds %s schema", data.MethodID, entity.name)

		return
	}

	var body payloadReader

	if body.data, err = reader.take(int(payloadSize)); err != nil {
		data.DecodeError = err.Error()

		return
	}

	var method rpcMethod = entity.clientMethods[data.MethodID]

	data.Method = method.name
	data.Arguments = make([]SemanticArgument, 0, len(method.arguments))
	for _, argument := range method.arguments {
		var semanticArgument SemanticArgument = SemanticArgument{Name: argument.name}

		if semanticArgument.Value, err = argument.kind.parseValue(&body); err != nil {
			data.DecodeError = fmt.Sprintf("%s: %v", method.name, err)

			break
		}

		data.Arguments = append(data.Arguments, semanticArgument)
	}
	data.Leftover = append(body.remaining(), reader.remaining()...)

	return
}

// decodeEntityProperty resolves and decodes one top-level property update.
func (decoder *SemanticDecoder) decodeEntityProperty(payload []byte) (data SemanticPropertyData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	data.PropertyID = reader.uint32()
	var payloadSize uint32 = reader.uint32()

	var (
		entity *entitySchema
		state  *semanticEntityState
		err    error
		found  bool
	)

	if state, found = decoder.entities[data.EntityID]; !found {
		data.DecodeError = fmt.Sprintf("unknown entity %d", data.EntityID)

		return
	}

	if entity, err = decoder.entity(state.entityType); err != nil {
		data.DecodeError = err.Error()

		return
	}
	data.EntityType = entity.name

	if int(data.PropertyID) >= len(entity.properties) {
		data.DecodeError = fmt.Sprintf("property %d exceeds %s schema", data.PropertyID, entity.name)

		return
	}
	data.Property = entity.properties[data.PropertyID].name

	var body payloadReader

	if body.data, err = reader.take(int(payloadSize)); err != nil {
		data.DecodeError = err.Error()

		return
	}

	if data.Value, err = entity.properties[data.PropertyID].kind.parseValue(&body); err != nil {
		data.DecodeError = fmt.Sprintf("%s: %v", data.Property, err)
	} else {
		state.properties[data.PropertyID] = data.Value
	}
	data.Leftover = append(body.remaining(), reader.remaining()...)

	return
}

// registerEntity initializes a full default property store for nested updates.
func (decoder *SemanticDecoder) registerEntity(entityID uint32, entityType uint16) {
	var state *semanticEntityState = &semanticEntityState{entityType: entityType}

	if entityType > 0 && int(entityType) <= len(decoder.schema.entities) {
		var properties []rpcProperty = decoder.schema.entities[int(entityType)-1].properties

		state.properties = make([]any, len(properties))
		for index := range properties {
			state.properties[index] = properties[index].kind.defaultValue()
		}
	}
	decoder.entities[entityID] = state
}

// entity validates BigWorld's one-based entity type index.
func (decoder *SemanticDecoder) entity(entityType uint16) (entity *entitySchema, err error) {
	if entityType == 0 || int(entityType) > len(decoder.schema.entities) {
		err = fmt.Errorf("entity type %d exceeds schema", entityType)

		return
	}

	entity = &decoder.schema.entities[int(entityType)-1]

	return
}

// decodeProperties decodes a dense ordered property list.
func decodeProperties(reader *payloadReader, properties []rpcProperty) (values map[string]any, err error) {
	values = make(map[string]any, len(properties))

	for _, property := range properties {
		if values[property.name], err = property.kind.parseValue(reader); err != nil {
			err = fmt.Errorf("%s: %w", property.name, err)

			return
		}
	}

	return
}

// remaining copies the unread portion of a payload.
func (reader *payloadReader) remaining() (remaining []byte) {
	if reader.offset < 0 || reader.offset >= len(reader.data) {
		return
	}

	remaining = make([]byte, len(reader.data)-reader.offset)
	copy(remaining, reader.data[reader.offset:])

	return
}

// take returns an exact number of bytes and advances the payload reader.
func (reader *payloadReader) take(length int) (value []byte, err error) {
	if length < 0 || reader.offset+length > len(reader.data) {
		reader.offset = len(reader.data) + 1
		err = io.ErrUnexpectedEOF

		return
	}

	value = reader.data[reader.offset : reader.offset+length]
	reader.offset += length

	return
}
