package wowsreplay

import (
	"errors"
	"fmt"
	"io"
)

type (
	bitReader struct {
		data      []byte
		bitOffset int
	}
)

// decodeNestedProperty resolves and applies one bit-packed nested update.
func (decoder *SemanticDecoder) decodeNestedProperty(payload []byte) (data SemanticNestedPropertyData) {
	var reader payloadReader = payloadReader{data: payload}

	data.EntityID = reader.uint32()
	var isSlice bool = reader.uint8()&1 != 0
	var payloadSize uint32 = reader.uint32()

	var (
		entity      *entitySchema
		err         error
		found       bool
		state       *semanticEntityState
		updateBytes []byte
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

	if updateBytes, err = reader.take(int(payloadSize)); err != nil || len(reader.remaining()) != 0 {
		data.DecodeError = "nested payload size mismatch"

		return
	}

	var bits bitReader = bitReader{data: updateBytes}
	var continuation uint64

	if continuation, err = bits.read(1); err != nil || continuation != 1 {
		data.DecodeError = "invalid nested property continuation"

		return
	}

	var propertyID uint64

	if propertyID, err = bits.read(bitWidth(len(entity.properties))); err != nil ||
		int(propertyID) >= len(entity.properties) {
		data.DecodeError = "nested property index is out of bounds"

		return
	}
	data.PropertyID = uint32(propertyID)
	data.Property = entity.properties[propertyID].name

	if data.Path, err = applyNestedUpdate(
		isSlice,
		entity.properties[propertyID].kind,
		&state.properties[propertyID],
		&bits,
		&data,
	); err != nil {
		data.DecodeError = err.Error()
	}

	return
}

// applyNestedUpdate navigates containers and applies the final action.
func applyNestedUpdate(
	isSlice bool,
	kind rpcType,
	value *any,
	reader *bitReader,
	data *SemanticNestedPropertyData,
) (path []any, err error) {
	var continuation uint64

	if continuation, err = reader.read(1); err != nil {
		return
	}

	if continuation == 0 {
		err = applyNestedAction(isSlice, kind, value, reader, data)

		return
	}

	switch kind.kind {
	case "fixed_dict":
		var index uint64

		if index, err = reader.read(bitWidth(len(kind.properties))); err != nil ||
			int(index) >= len(kind.properties) {
			err = errors.New("nested dictionary index is out of bounds")

			return
		}

		var field rpcField = kind.properties[index]

		if field.kind.kind == "primitive" {
			var parsed any

			if parsed, err = readAlignedValue(field.kind, reader); err != nil {
				return
			}

			var values map[string]any = ensureMap(value, kind)

			values[field.name] = parsed
			data.Action = "set_key"
			data.Key = field.name
			data.Value = parsed

			return
		}

		var values map[string]any = ensureMap(value, kind)
		var child any = values[field.name]

		if child == nil {
			child = field.kind.defaultValue()
		}

		if path, err = applyNestedUpdate(isSlice, field.kind, &child, reader, data); err == nil {
			values[field.name] = child
			path = append([]any{field.name}, path...)
		}
	case "array", "tuple":
		var values []any = ensureSlice(value)
		var index uint64

		if index, err = reader.read(bitWidth(len(values))); err != nil {
			return
		}

		for len(values) <= int(index) {
			values = append(values, kind.element.defaultValue())
		}
		*value = values

		if kind.element.kind == "primitive" {
			var parsed any

			if parsed, err = readAlignedValue(*kind.element, reader); err != nil {
				return
			}

			values[index] = parsed
			data.Action = "set_element"
			data.Index = int(index)
			data.Value = parsed

			return
		}

		var child any = values[index]

		if path, err = applyNestedUpdate(isSlice, *kind.element, &child, reader, data); err == nil {
			values[index] = child
			path = append([]any{int(index)}, path...)
		}
	default:
		err = errors.New("nested path descends into a scalar")
	}

	return
}

// applyNestedAction decodes a set, insertion, replacement, or removal.
func applyNestedAction(
	isSlice bool,
	kind rpcType,
	value *any,
	reader *bitReader,
	data *SemanticNestedPropertyData,
) (err error) {
	switch kind.kind {
	case "fixed_dict":
		var index uint64

		if index, err = reader.read(bitWidth(len(kind.properties))); err != nil ||
			int(index) >= len(kind.properties) {
			err = errors.New("nested dictionary action index is out of bounds")

			return
		}

		var field rpcField = kind.properties[index]
		var parsed any

		if parsed, err = readAlignedValue(field.kind, reader); err != nil {
			return
		}

		var values map[string]any = ensureMap(value, kind)

		values[field.name] = parsed
		data.Action = "set_key"
		data.Key = field.name
		data.Value = parsed
	case "array", "tuple":
		var values []any = ensureSlice(value)
		var width uint8 = bitWidth(len(values))

		if isSlice {
			width = bitWidth(len(values) + 1)
		}

		var start uint64

		if start, err = reader.read(width); err != nil {
			return
		}

		var stop uint64 = start

		if isSlice {
			if stop, err = reader.read(width); err != nil {
				return
			}
		}

		if err = reader.align(); err != nil {
			return
		}

		var remaining []byte = reader.remainingBytes()

		if isSlice && len(remaining) == 0 {
			values = replaceSlice(values, int(start), int(stop), nil)
			*value = values
			data.Action = "remove_range"
			data.Start = int(start)
			data.Stop = int(stop)

			return
		}

		var (
			payloadReader payloadReader = payloadReader{data: remaining}
			replacements  []any
		)

		for payloadReader.offset < len(payloadReader.data) {
			var before int = payloadReader.offset
			var parsed any

			if parsed, err = kind.element.parseValue(&payloadReader); err != nil ||
				payloadReader.offset <= before {
				err = errors.New("nested array element payload is malformed")

				return
			}
			replacements = append(replacements, parsed)

			if !isSlice {
				break
			}
		}

		if isSlice {
			values = replaceSlice(values, int(start), int(stop), replacements)
			*value = values
			data.Action = "set_range"
			data.Start = int(start)
			data.Stop = int(stop)
			data.Values = replacements
		} else {
			for len(values) <= int(start) {
				values = append(values, kind.element.defaultValue())
			}
			values[start] = replacements[0]
			*value = values
			data.Action = "set_element"
			data.Index = int(start)
			data.Value = replacements[0]
		}
	default:
		err = errors.New("nested action targets a scalar")
	}

	return
}

// readAlignedValue parses a schema value from the remaining whole bytes.
func readAlignedValue(kind rpcType, reader *bitReader) (value any, err error) {
	if err = reader.align(); err != nil {
		return
	}

	var payload payloadReader = payloadReader{data: reader.remainingBytes()}

	if value, err = kind.parseValue(&payload); err != nil {
		return
	}
	reader.bitOffset += payload.offset * 8

	return
}

// ensureMap materializes a fixed dictionary's defaults.
func ensureMap(value *any, kind rpcType) (values map[string]any) {
	var found bool

	if values, found = (*value).(map[string]any); !found {
		values, _ = kind.defaultValue().(map[string]any)
		*value = values
	}

	return
}

// ensureSlice materializes an empty array when a property was not initialized.
func ensureSlice(value *any) (values []any) {
	var found bool

	if values, found = (*value).([]any); !found {
		values = []any{}
		*value = values
	}

	return
}

// replaceSlice applies Python-style half-open slice replacement.
func replaceSlice(values []any, start, stop int, replacements []any) (updated []any) {
	start = min(max(start, 0), len(values))
	stop = min(max(stop, start), len(values))
	updated = make([]any, 0, len(values)-(stop-start)+len(replacements))
	updated = append(updated, values[:start]...)
	updated = append(updated, replacements...)
	updated = append(updated, values[stop:]...)

	return
}

// bitWidth returns the number of index bits used for a container length.
func bitWidth(length int) (width uint8) {
	var power int = 1

	for power < length {
		power <<= 1
		width++
	}

	return
}

// read consumes bits most-significant-bit first.
func (reader *bitReader) read(count uint8) (value uint64, err error) {
	if reader.bitOffset+int(count) > len(reader.data)*8 {
		err = io.ErrUnexpectedEOF

		return
	}

	for bit := uint8(0); bit < count; bit++ {
		var (
			byteIndex int = reader.bitOffset / 8
			bitIndex  int = 7 - reader.bitOffset%8
		)

		value = value<<1 | uint64((reader.data[byteIndex]>>bitIndex)&1)
		reader.bitOffset++
	}

	return
}

// align advances to the next byte boundary.
func (reader *bitReader) align() (err error) {
	if remainder := reader.bitOffset % 8; remainder != 0 {
		reader.bitOffset += 8 - remainder
	}

	if reader.bitOffset > len(reader.data)*8 {
		err = io.ErrUnexpectedEOF
	}

	return
}

// remainingBytes returns unread byte-aligned data.
func (reader *bitReader) remainingBytes() (remaining []byte) {
	if reader.bitOffset%8 == 0 && reader.bitOffset <= len(reader.data)*8 {
		remaining = reader.data[reader.bitOffset/8:]
	}

	return
}
