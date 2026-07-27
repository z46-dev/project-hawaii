package wowsreplay

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const variableRPCSize int = 0xffff

type (
	rpcPrimitive uint8

	rpcType struct {
		kind       string
		name       string
		primitive  rpcPrimitive
		element    *rpcType
		count      int
		allowNone  bool
		properties []rpcField
		userType   bool
	}

	rpcField struct {
		name string
		kind rpcType
	}

	rpcProperty struct {
		name  string
		kind  rpcType
		flags string
	}

	rpcMethod struct {
		name                     string
		arguments                []rpcField
		variableLengthHeaderSize int
	}

	entitySchema struct {
		name               string
		properties         []rpcProperty
		internalProperties []rpcProperty
		baseProperties     []rpcProperty
		clientMethods      []rpcMethod
	}

	replaySchema struct {
		entities []entitySchema
	}

	entityDefinition struct {
		properties    []rpcProperty
		clientMethods []rpcMethod
		implements    []string
	}

	xmlNode struct {
		name     string
		text     string
		children []*xmlNode
	}
)

const (
	rpcUint8 rpcPrimitive = iota
	rpcUint16
	rpcUint32
	rpcUint64
	rpcInt8
	rpcInt16
	rpcInt32
	rpcInt64
	rpcFloat32
	rpcFloat64
	rpcVector2
	rpcVector3
	rpcString
	rpcUnicodeString
	rpcBlob
)

// loadReplaySchema loads the entity protocol schema from an imported resource bundle.
func loadReplaySchema(bundle *ResourceBundle) (schema replaySchema, err error) {
	var (
		aliases      map[string]rpcType
		entitiesNode *xmlNode
		entitiesRoot *xmlNode
	)

	if aliases, err = loadAliases(bundle); err != nil {
		return
	}

	if entitiesRoot, err = loadXMLFromBundle(bundle, "scripts/entities.xml"); err != nil {
		return
	}

	if entitiesNode = entitiesRoot.child("ClientServerEntities"); entitiesNode == nil {
		err = errors.New("scripts/entities.xml has no ClientServerEntities section")

		return
	}

	for _, entityNode := range entitiesNode.children {
		var (
			definition entityDefinition
			entity     entitySchema
		)

		if definition, err = loadEntityDefinition(bundle, entityNode.name, aliases, make(map[string]bool)); err != nil {
			err = fmt.Errorf("load entity %s: %w", entityNode.name, err)

			return
		}

		entity.name = entityNode.name
		entity.baseProperties = filterProperties(definition.properties, map[string]bool{
			"BASE_AND_CLIENT": true,
		})
		entity.internalProperties = filterProperties(definition.properties, map[string]bool{
			"ALL_CLIENTS":         true,
			"CELL_PUBLIC_AND_OWN": true,
			"OWN_CLIENT":          true,
			"OTHER_CLIENTS":       true,
		})
		entity.properties = filterProperties(definition.properties, map[string]bool{
			"ALL_CLIENTS":         true,
			"BASE_AND_CLIENT":     true,
			"CELL_PUBLIC_AND_OWN": true,
			"OWN_CLIENT":          true,
			"OTHER_CLIENTS":       true,
		})
		sort.SliceStable(entity.properties, func(left, right int) bool {
			return entity.properties[left].kind.sortSize() < entity.properties[right].kind.sortSize()
		})

		entity.clientMethods = deduplicateMethods(definition.clientMethods)
		sort.SliceStable(entity.clientMethods, func(left, right int) bool {
			return entity.clientMethods[left].sortSize() < entity.clientMethods[right].sortSize()
		})
		schema.entities = append(schema.entities, entity)
	}

	return
}

// loadAliases parses aliases in declaration order because later types may reference earlier ones.
func loadAliases(bundle *ResourceBundle) (aliases map[string]rpcType, err error) {
	var root *xmlNode

	if root, err = loadXMLFromBundle(bundle, "scripts/entity_defs/alias.xml"); err != nil {
		return
	}

	aliases = make(map[string]rpcType, len(root.children))
	for _, node := range root.children {
		var parsed rpcType

		if parsed, err = parseRPCType(node, aliases); err != nil {
			err = fmt.Errorf("parse alias %s: %w", node.name, err)

			return
		}

		parsed.name = node.name
		aliases[node.name] = parsed
	}

	return
}

// loadEntityDefinition recursively merges implemented interfaces before an entity's own fields.
func loadEntityDefinition(
	bundle *ResourceBundle,
	name string,
	aliases map[string]rpcType,
	visiting map[string]bool,
) (definition entityDefinition, err error) {
	var (
		node *xmlNode
		path string
	)

	if visiting[name] {
		err = fmt.Errorf("cyclic interface inheritance involving %s", name)

		return
	}
	visiting[name] = true
	defer delete(visiting, name)

	if strings.HasPrefix(name, "interfaces/") {
		path = "scripts/entity_defs/" + name + ".def"
	} else {
		path = "scripts/entity_defs/" + name + ".def"
	}

	if node, err = loadXMLFromBundle(bundle, path); err != nil {
		return
	}

	if implements := node.child("Implements"); implements != nil {
		for _, interfaceNode := range implements.children {
			var inherited entityDefinition
			var interfaceName string = strings.TrimSpace(interfaceNode.text)

			if interfaceName == "" {
				continue
			}

			if inherited, err = loadEntityDefinition(
				bundle,
				"interfaces/"+interfaceName,
				aliases,
				visiting,
			); err != nil {
				return
			}

			definition.properties = append(definition.properties, inherited.properties...)
			definition.clientMethods = append(definition.clientMethods, inherited.clientMethods...)
		}
	}

	if properties := node.child("Properties"); properties != nil {
		for _, propertyNode := range properties.children {
			var (
				flagsNode *xmlNode = propertyNode.child("Flags")
				typeNode  *xmlNode = propertyNode.child("Type")
				property  rpcProperty
			)

			if flagsNode == nil || typeNode == nil {
				continue
			}

			property.name = propertyNode.name
			property.flags = strings.TrimSpace(flagsNode.text)
			if property.kind, err = parseRPCType(typeNode, aliases); err != nil {
				err = fmt.Errorf("parse property %s: %w", property.name, err)

				return
			}

			definition.properties = append(definition.properties, property)
		}
	}

	if methods := node.child("ClientMethods"); methods != nil {
		for _, methodNode := range methods.children {
			var method rpcMethod = rpcMethod{
				name:                     methodNode.name,
				variableLengthHeaderSize: 1,
			}

			if headerNode := methodNode.child("VariableLengthHeaderSize"); headerNode != nil {
				if method.variableLengthHeaderSize, err = strconv.Atoi(strings.TrimSpace(headerNode.text)); err != nil {
					method.variableLengthHeaderSize = 1
					err = nil
				}
			}

			for _, argumentNode := range methodNode.children {
				if argumentNode.name == "Arg" {
					var field rpcField

					if field.kind, err = parseRPCType(argumentNode, aliases); err != nil {
						return
					}

					method.arguments = append(method.arguments, field)
				} else if argumentNode.name == "Args" {
					for _, namedArgumentNode := range argumentNode.children {
						var field rpcField = rpcField{name: namedArgumentNode.name}

						if field.kind, err = parseRPCType(namedArgumentNode, aliases); err != nil {
							return
						}

						method.arguments = append(method.arguments, field)
					}
				}
			}

			definition.clientMethods = append(definition.clientMethods, method)
		}
	}

	return
}

// parseRPCType converts a definition XML node into its recursive wire type.
func parseRPCType(node *xmlNode, aliases map[string]rpcType) (kind rpcType, err error) {
	var name string = node.firstText()

	switch name {
	case "UINT8", "BOOL":
		kind = primitiveRPCType(rpcUint8)
	case "UINT16":
		kind = primitiveRPCType(rpcUint16)
	case "UINT32":
		kind = primitiveRPCType(rpcUint32)
	case "UINT64":
		kind = primitiveRPCType(rpcUint64)
	case "INT8":
		kind = primitiveRPCType(rpcInt8)
	case "INT16":
		kind = primitiveRPCType(rpcInt16)
	case "INT32":
		kind = primitiveRPCType(rpcInt32)
	case "INT64":
		kind = primitiveRPCType(rpcInt64)
	case "FLOAT", "FLOAT32":
		kind = primitiveRPCType(rpcFloat32)
	case "FLOAT64":
		kind = primitiveRPCType(rpcFloat64)
	case "VECTOR2":
		kind = primitiveRPCType(rpcVector2)
	case "VECTOR3":
		kind = primitiveRPCType(rpcVector3)
	case "STRING":
		kind = primitiveRPCType(rpcString)
	case "UNICODE_STRING":
		kind = primitiveRPCType(rpcUnicodeString)
	case "BLOB", "MAILBOX", "PYTHON":
		kind = primitiveRPCType(rpcBlob)
	case "USER_TYPE":
		kind.userType = true
		if typeNode := node.child("Type"); typeNode != nil {
			if kind, err = parseRPCType(typeNode, aliases); err != nil {
				return
			}
			kind.userType = true
		} else {
			kind = primitiveRPCType(rpcBlob)
			kind.userType = true
		}
	case "ARRAY", "TUPLE":
		var elementNode *xmlNode = node.child("of")

		if elementNode == nil {
			err = fmt.Errorf("%s has no element type", name)

			return
		}

		kind.kind = strings.ToLower(name)
		kind.element = &rpcType{}
		if *kind.element, err = parseRPCType(elementNode, aliases); err != nil {
			return
		}

		kind.count = -1
		if sizeNode := node.child("size"); sizeNode != nil {
			if kind.count, err = strconv.Atoi(strings.TrimSpace(sizeNode.text)); err != nil {
				return
			}
		}
	case "FIXED_DICT":
		kind.kind = "fixed_dict"
		kind.allowNone = node.child("AllowNone") != nil
		if propertiesNode := node.child("Properties"); propertiesNode != nil {
			for _, propertyNode := range propertiesNode.children {
				var (
					field    rpcField = rpcField{name: propertyNode.name}
					typeNode *xmlNode = propertyNode.child("Type")
				)

				if typeNode == nil {
					err = fmt.Errorf("fixed dictionary field %s has no Type", field.name)

					return
				}

				if field.kind, err = parseRPCType(typeNode, aliases); err != nil {
					return
				}

				kind.properties = append(kind.properties, field)
			}
		}
	default:
		var found bool

		if kind, found = aliases[name]; !found {
			err = fmt.Errorf("unknown RPC type %q", name)

			return
		}

		kind.name = name
	}

	return
}

// parseValue reads one schema value from a packet payload.
func (kind rpcType) parseValue(reader *payloadReader) (value any, err error) {
	if kind.userType {
		kind.userType = false
	}

	switch kind.kind {
	case "primitive":
		value, err = kind.parsePrimitive(reader)
	case "array", "tuple":
		var count int = kind.count

		if count < 0 {
			count = int(reader.uint8())
		}

		if reader.offset > len(reader.data) {
			err = io.ErrUnexpectedEOF

			return
		}

		var values []any = make([]any, 0, count)

		for index := 0; index < count; index++ {
			var item any

			if item, err = kind.element.parseValue(reader); err != nil {
				return
			}

			values = append(values, item)
		}
		value = values
	case "fixed_dict":
		if kind.allowNone {
			var present uint8 = reader.uint8()

			if present == 0 {
				return
			}

			if present != 1 {
				err = fmt.Errorf("invalid nullable fixed dictionary flag %d", present)

				return
			}
		}

		var values map[string]any = make(map[string]any, len(kind.properties))

		for _, property := range kind.properties {
			if values[property.name], err = property.kind.parseValue(reader); err != nil {
				return
			}
		}
		value = values
	default:
		err = fmt.Errorf("unsupported RPC type kind %q", kind.kind)
	}

	if reader.offset > len(reader.data) && err == nil {
		err = io.ErrUnexpectedEOF
	}

	return
}

// parsePrimitive reads one fixed or length-prefixed primitive value.
func (kind rpcType) parsePrimitive(reader *payloadReader) (value any, err error) {
	switch kind.primitive {
	case rpcUint8:
		value = reader.uint8()
	case rpcUint16:
		value = reader.uint16()
	case rpcUint32:
		value = reader.uint32()
	case rpcUint64:
		value = reader.uint64()
	case rpcInt8:
		value = int8(reader.uint8())
	case rpcInt16:
		value = reader.int16()
	case rpcInt32:
		value = reader.int32()
	case rpcInt64:
		value = reader.int64()
	case rpcFloat32:
		value = reader.float32()
	case rpcFloat64:
		value = reader.float64()
	case rpcVector2:
		value = []float32{reader.float32(), reader.float32()}
	case rpcVector3:
		value = reader.vector3()
	case rpcString, rpcUnicodeString, rpcBlob:
		var data []byte

		if data, err = reader.bytes8(); err != nil {
			return
		}

		if kind.primitive == rpcBlob {
			value = data
		} else {
			value = string(data)
		}
	default:
		err = errors.New("unknown RPC primitive")
	}

	return
}

// sortSize reports the BigWorld exposed-method ordering size.
func (kind rpcType) sortSize() (size int) {
	if kind.userType {
		size = variableRPCSize

		return
	}

	switch kind.kind {
	case "primitive":
		switch kind.primitive {
		case rpcUint8, rpcInt8:
			size = 1
		case rpcUint16, rpcInt16:
			size = 2
		case rpcUint32, rpcInt32, rpcFloat32:
			size = 4
		case rpcUint64, rpcInt64, rpcFloat64:
			size = 8
		case rpcVector2:
			size = 8
		case rpcVector3:
			size = 12
		default:
			size = variableRPCSize
		}
	case "array", "tuple":
		if kind.count < 0 {
			size = variableRPCSize
		} else if size = kind.element.sortSize(); size != variableRPCSize {
			size *= kind.count
		}
	case "fixed_dict":
		if kind.allowNone {
			size = variableRPCSize

			return
		}

		for _, property := range kind.properties {
			var fieldSize int = property.kind.sortSize()

			if fieldSize == variableRPCSize || size > variableRPCSize-fieldSize {
				size = variableRPCSize

				return
			}
			size += fieldSize
		}
	default:
		size = variableRPCSize
	}

	return
}

// defaultValue materializes a writable zero value for nested property updates.
func (kind rpcType) defaultValue() (value any) {
	switch kind.kind {
	case "primitive":
		switch kind.primitive {
		case rpcUint8:
			value = uint8(0)
		case rpcUint16:
			value = uint16(0)
		case rpcUint32:
			value = uint32(0)
		case rpcUint64:
			value = uint64(0)
		case rpcInt8:
			value = int8(0)
		case rpcInt16:
			value = int16(0)
		case rpcInt32:
			value = int32(0)
		case rpcInt64:
			value = int64(0)
		case rpcFloat32:
			value = float32(0)
		case rpcFloat64:
			value = float64(0)
		case rpcVector2:
			value = []float32{0, 0}
		case rpcVector3:
			value = Vector3{}
		case rpcString, rpcUnicodeString:
			value = ""
		case rpcBlob:
			value = []byte{}
		}
	case "array", "tuple":
		var count int = max(kind.count, 0)
		var values []any = make([]any, count)

		for index := range values {
			values[index] = kind.element.defaultValue()
		}
		value = values
	case "fixed_dict":
		var values map[string]any = make(map[string]any, len(kind.properties))

		for _, property := range kind.properties {
			values[property.name] = property.kind.defaultValue()
		}
		value = values
	}

	return
}

// sortSize reports an exposed method's ordering key.
func (method rpcMethod) sortSize() (size int) {
	for _, argument := range method.arguments {
		var argumentSize int = argument.kind.sortSize()

		if argumentSize == variableRPCSize || size > variableRPCSize-argumentSize {
			size = variableRPCSize + method.variableLengthHeaderSize

			return
		}
		size += argumentSize
	}
	size += method.variableLengthHeaderSize

	return
}

// loadXMLFromBundle parses one imported VFS XML document.
func loadXMLFromBundle(bundle *ResourceBundle, path string) (root *xmlNode, err error) {
	var (
		file     *os.File
		filePath string
	)

	if filePath, err = bundle.VFSPath(path); err != nil {
		return
	}

	if file, err = os.Open(filePath); err != nil {
		return
	}
	defer file.Close()

	var decoder *xml.Decoder = xml.NewDecoder(file)

	for {
		var token xml.Token

		if token, err = decoder.Token(); err != nil {
			return
		}

		if start, ok := token.(xml.StartElement); ok {
			root, err = decodeXMLNode(decoder, start)

			return
		}
	}
}

// decodeXMLNode constructs a compact XML tree while retaining mixed type text.
func decodeXMLNode(decoder *xml.Decoder, start xml.StartElement) (node *xmlNode, err error) {
	node = &xmlNode{name: start.Name.Local}

	for {
		var token xml.Token

		if token, err = decoder.Token(); err != nil {
			return
		}

		switch value := token.(type) {
		case xml.StartElement:
			var child *xmlNode

			if child, err = decodeXMLNode(decoder, value); err != nil {
				return
			}
			node.children = append(node.children, child)
		case xml.CharData:
			node.text += string(value)
		case xml.EndElement:
			return
		}
	}
}

// child returns the first direct child with a case-sensitive tag name.
func (node *xmlNode) child(name string) (child *xmlNode) {
	for _, candidate := range node.children {
		if candidate.name == name {
			child = candidate

			return
		}
	}

	return
}

// firstText returns the leading schema keyword from a node's mixed text.
func (node *xmlNode) firstText() (value string) {
	var fields []string = strings.Fields(node.text)

	if len(fields) > 0 {
		value = fields[0]
	}

	return
}

// primitiveRPCType constructs a primitive schema type.
func primitiveRPCType(primitive rpcPrimitive) (kind rpcType) {
	kind = rpcType{kind: "primitive", primitive: primitive}

	return
}

// filterProperties selects client-visible properties while preserving definition order.
func filterProperties(properties []rpcProperty, allowed map[string]bool) (filtered []rpcProperty) {
	for _, property := range properties {
		if allowed[property.flags] {
			filtered = append(filtered, property)
		}
	}

	return
}

// deduplicateMethods mirrors the engine's first-definition-wins behavior.
func deduplicateMethods(methods []rpcMethod) (deduplicated []rpcMethod) {
	var seen map[string]bool = make(map[string]bool, len(methods))

	for _, method := range methods {
		if !seen[method.name] {
			deduplicated = append(deduplicated, method)
			seen[method.name] = true
		}
	}

	return
}

// uint64 reads a little-endian unsigned integer and records underflow.
func (reader *payloadReader) uint64() (value uint64) {
	if reader.offset+8 > len(reader.data) {
		reader.offset = len(reader.data) + 1

		return
	}

	value = uint64(reader.int64())

	return
}

// bytes8 reads BigWorld's compact length-prefixed byte representation.
func (reader *payloadReader) bytes8() (value []byte, err error) {
	var length int = int(reader.uint8())

	if length == 0xff {
		length = int(reader.uint16())
		reader.uint8()
	}

	if reader.offset+length > len(reader.data) {
		reader.offset = len(reader.data) + 1
		err = io.ErrUnexpectedEOF

		return
	}

	value = make([]byte, length)
	copy(value, reader.data[reader.offset:reader.offset+length])
	reader.offset += length

	return
}
