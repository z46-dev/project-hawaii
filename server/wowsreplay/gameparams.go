package wowsreplay

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/nlpodyssey/gopickle/pickle"
	"github.com/nlpodyssey/gopickle/types"
)

var gameParameterCache sync.Map

type (
	// ShipParameters contains the renderer-facing subset of one GameParams ship.
	ShipParameters struct {
		ID      uint64 `json:"id"`
		Index   string `json:"index,omitempty"`
		Name    string `json:"name,omitempty"`
		Species string `json:"species,omitempty"`
		Nation  string `json:"nation,omitempty"`
	}

	// MapBorderParameters describes one mode-specific playable map viewport.
	MapBorderParameters struct {
		ID     uint64  `json:"id"`
		Width  float32 `json:"width"`
		Height float32 `json:"height"`
	}

	// ProjectileParameters contains renderer-facing range data for ammunition.
	ProjectileParameters struct {
		ID          uint64  `json:"id"`
		Species     string  `json:"species,omitempty"`
		Speed       float32 `json:"speed,omitempty"`
		MaxDistance float32 `json:"maxDistance,omitempty"`
	}

	gameParameterCatalog struct {
		ships       map[uint64]ShipParameters
		mapBorders  map[uint64]MapBorderParameters
		projectiles map[uint64]ProjectileParameters
	}

	gameParamsPickleClass struct{}

	gameParamsPickleObject struct {
		dictionary types.Dict
	}
)

// LoadShipParameters reads ship identities and classes from the imported GameParams data.
func (bundle *ResourceBundle) LoadShipParameters() (ships map[uint64]ShipParameters, err error) {
	var catalog *gameParameterCatalog

	if catalog, err = bundle.loadGameParameterCatalog(); err == nil {
		ships = catalog.ships
	}

	return
}

// LoadMapBorderParameters reads mode-specific playable viewport sizes.
func (bundle *ResourceBundle) LoadMapBorderParameters() (borders map[uint64]MapBorderParameters, err error) {
	var catalog *gameParameterCatalog

	if catalog, err = bundle.loadGameParameterCatalog(); err == nil {
		borders = catalog.mapBorders
	}

	return
}

// LoadProjectileParameters reads ammunition ranges from imported GameParams data.
func (bundle *ResourceBundle) LoadProjectileParameters() (projectiles map[uint64]ProjectileParameters, err error) {
	var catalog *gameParameterCatalog

	if catalog, err = bundle.loadGameParameterCatalog(); err == nil {
		projectiles = catalog.projectiles
	}

	return
}

// loadGameParameterCatalog decodes and caches the renderer-facing GameParams subset.
func (bundle *ResourceBundle) loadGameParameterCatalog() (catalog *gameParameterCatalog, err error) {
	var (
		compressed []byte
		cached     any
		cacheFound bool
		path       string
		reader     io.ReadCloser
		root       any
		unpickler  pickle.Unpickler
	)

	if cached, cacheFound = gameParameterCache.Load(bundle.Root); cacheFound {
		catalog = cached.(*gameParameterCatalog)

		return
	}

	if path, err = bundle.VFSPath("content/GameParams.data"); err != nil {
		return
	}

	if compressed, err = os.ReadFile(path); err != nil {
		err = fmt.Errorf("read GameParams: %w", err)

		return
	}

	reverseBytes(compressed)
	if reader, err = zlib.NewReader(bytes.NewReader(compressed)); err != nil {
		err = fmt.Errorf("decompress GameParams: %w", err)

		return
	}
	defer reader.Close()

	unpickler = pickle.NewUnpickler(reader)
	unpickler.FindClass = func(_, _ string) (class any, findErr error) {
		class = &gameParamsPickleClass{}

		return
	}
	if root, err = unpickler.Load(); err != nil {
		err = fmt.Errorf("decode GameParams pickle: %w", err)

		return
	}

	catalog = &gameParameterCatalog{
		ships:       make(map[uint64]ShipParameters),
		mapBorders:  make(map[uint64]MapBorderParameters),
		projectiles: make(map[uint64]ProjectileParameters),
	}
	collectGameParameters(
		root,
		catalog,
		make(map[*types.Dict]bool),
		make(map[*gameParamsPickleObject]bool),
	)
	gameParameterCache.Store(bundle.Root, catalog)

	return
}

// reverseBytes restores the reversed zlib stream used by GameParams.data.
func reverseBytes(data []byte) {
	for left, right := 0, len(data)-1; left < right; left, right = left+1, right-1 {
		data[left], data[right] = data[right], data[left]
	}
}

// collectGameParameters recursively locates renderer-facing parameter dictionaries.
func collectGameParameters(
	value any,
	catalog *gameParameterCatalog,
	visitedDictionaries map[*types.Dict]bool,
	visitedObjects map[*gameParamsPickleObject]bool,
) {
	switch typed := value.(type) {
	case *types.Dict:
		var (
			border          MapBorderParameters
			borderFound     bool
			projectile      ProjectileParameters
			projectileFound bool
			ship            ShipParameters
			shipFound       bool
		)

		if visitedDictionaries[typed] {
			return
		}
		visitedDictionaries[typed] = true

		if ship, shipFound = shipParametersFromDict(typed); shipFound {
			catalog.ships[ship.ID] = ship
		}
		if border, borderFound = mapBorderParametersFromDict(typed); borderFound {
			catalog.mapBorders[border.ID] = border
		}
		if projectile, projectileFound = projectileParametersFromDict(typed); projectileFound {
			catalog.projectiles[projectile.ID] = projectile
		}

		for _, entry := range *typed {
			collectGameParameters(entry.Value, catalog, visitedDictionaries, visitedObjects)
		}
	case *types.List:
		for _, item := range *typed {
			collectGameParameters(item, catalog, visitedDictionaries, visitedObjects)
		}
	case *types.Tuple:
		for _, item := range *typed {
			collectGameParameters(item, catalog, visitedDictionaries, visitedObjects)
		}
	case *gameParamsPickleObject:
		var (
			border          MapBorderParameters
			borderFound     bool
			projectile      ProjectileParameters
			projectileFound bool
			ship            ShipParameters
			shipFound       bool
		)

		if visitedObjects[typed] {
			return
		}
		visitedObjects[typed] = true

		if ship, shipFound = shipParametersFromDict(&typed.dictionary); shipFound {
			catalog.ships[ship.ID] = ship
		}
		if border, borderFound = mapBorderParametersFromDict(&typed.dictionary); borderFound {
			catalog.mapBorders[border.ID] = border
		}
		if projectile, projectileFound = projectileParametersFromDict(&typed.dictionary); projectileFound {
			catalog.projectiles[projectile.ID] = projectile
		}
		collectGameParameters(&typed.dictionary, catalog, visitedDictionaries, visitedObjects)
	}
}

// shipParametersFromDict recognizes the stable id/index/typeinfo ship shape.
func shipParametersFromDict(dictionary *types.Dict) (ship ShipParameters, found bool) {
	var (
		idValue       any
		indexValue    any
		nameValue     any
		nationValue   any
		typeInfoValue any
		typeInfo      *types.Dict
		typeValue     any
		speciesValue  any
	)

	if idValue, found = pickleDictionaryValue(dictionary, "id"); !found {
		return
	}

	if typeInfoValue, found = pickleDictionaryValue(dictionary, "typeinfo"); !found {
		return
	}

	if typeInfo, found = pickleDictionary(typeInfoValue); !found {
		return
	}

	if typeValue, found = pickleDictionaryValue(typeInfo, "type"); !found || stringValue(typeValue) != "Ship" {
		found = false

		return
	}

	ship.ID = pickleID(idValue)
	if ship.ID == 0 {
		found = false

		return
	}

	if indexValue, found = pickleDictionaryValue(dictionary, "index"); found {
		ship.Index = stringValue(indexValue)
	}

	if nameValue, found = pickleDictionaryValue(dictionary, "name"); found {
		ship.Name = stringValue(nameValue)
	}

	if speciesValue, found = pickleDictionaryValue(typeInfo, "species"); found {
		ship.Species = stringValue(speciesValue)
	}
	if nationValue, found = pickleDictionaryValue(typeInfo, "nation"); found {
		ship.Nation = stringValue(nationValue)
	}

	found = true

	return
}

// mapBorderParametersFromDict recognizes GameParams map-border dimensions.
func mapBorderParametersFromDict(dictionary *types.Dict) (border MapBorderParameters, found bool) {
	var (
		heightValue   any
		idValue       any
		typeInfo      *types.Dict
		typeInfoValue any
		typeValue     any
		speciesValue  any
		widthValue    any
	)

	if idValue, found = pickleDictionaryValue(dictionary, "id"); !found {
		return
	}
	if typeInfoValue, found = pickleDictionaryValue(dictionary, "typeinfo"); !found {
		return
	}
	if typeInfo, found = pickleDictionary(typeInfoValue); !found {
		return
	}
	if typeValue, found = pickleDictionaryValue(typeInfo, "type"); !found || stringValue(typeValue) != "Other" {
		found = false

		return
	}
	if speciesValue, found = pickleDictionaryValue(typeInfo, "species"); !found ||
		stringValue(speciesValue) != "MapBorder" {
		found = false

		return
	}
	if widthValue, found = pickleDictionaryValue(dictionary, "width"); !found {
		return
	}
	if heightValue, found = pickleDictionaryValue(dictionary, "height"); !found {
		return
	}

	border = MapBorderParameters{
		ID:     pickleID(idValue),
		Width:  float32(numericValue(widthValue)),
		Height: float32(numericValue(heightValue)),
	}
	found = border.ID != 0 && border.Width > 0 && border.Height > 0

	return
}

// projectileParametersFromDict recognizes ranged ammunition parameter records.
func projectileParametersFromDict(dictionary *types.Dict) (projectile ProjectileParameters, found bool) {
	var (
		distanceValue any
		idValue       any
		speciesValue  any
		speedValue    any
		typeInfo      *types.Dict
		typeInfoValue any
		typeValue     any
	)

	if idValue, found = pickleDictionaryValue(dictionary, "id"); !found {
		return
	}
	if typeInfoValue, found = pickleDictionaryValue(dictionary, "typeinfo"); !found {
		return
	}
	if typeInfo, found = pickleDictionary(typeInfoValue); !found {
		return
	}
	if typeValue, found = pickleDictionaryValue(typeInfo, "type"); !found ||
		stringValue(typeValue) != "Projectile" {
		found = false

		return
	}

	projectile.ID = pickleID(idValue)
	if projectile.ID == 0 {
		found = false

		return
	}
	if speciesValue, found = pickleDictionaryValue(typeInfo, "species"); found {
		projectile.Species = stringValue(speciesValue)
	}
	if speedValue, found = pickleDictionaryValue(dictionary, "speed"); found {
		projectile.Speed = float32(numericValue(speedValue))
	}
	if distanceValue, found = pickleDictionaryValue(dictionary, "maxDist"); found {
		projectile.MaxDistance = float32(numericValue(distanceValue))
	}
	found = true

	return
}

// pickleID converts the integer forms emitted by the pickle decoder.
func pickleID(value any) (identifier uint64) {
	switch typed := value.(type) {
	case int:
		identifier = uint64(typed)
	case int64:
		identifier = uint64(typed)
	case uint64:
		identifier = typed
	}

	return
}

// pickleDictionary exposes plain dictionaries and harmless decoded helper objects uniformly.
func pickleDictionary(value any) (dictionary *types.Dict, found bool) {
	switch typed := value.(type) {
	case *types.Dict:
		dictionary = typed
		found = true
	case *gameParamsPickleObject:
		dictionary = &typed.dictionary
		found = true
	}

	return
}

// pickleDictionaryValue accepts both Unicode and Python 2 byte-string keys.
func pickleDictionaryValue(dictionary *types.Dict, name string) (value any, found bool) {
	for _, entry := range *dictionary {
		if stringValue(entry.Key) == name {
			value = entry.Value
			found = true

			return
		}
	}

	return
}

// PyNew constructs a harmless state container for GameParams helper classes.
func (class *gameParamsPickleClass) PyNew(_ ...any) (object any, err error) {
	object = &gameParamsPickleObject{}

	return
}

// Call supports pickle REDUCE operations for GameParams helper classes.
func (class *gameParamsPickleClass) Call(_ ...any) (object any, err error) {
	object = &gameParamsPickleObject{}

	return
}

// PyDictSet records object state without executing imported Python behavior.
func (object *gameParamsPickleObject) PyDictSet(key, value any) (err error) {
	object.dictionary.Set(key, value)

	return
}

// stringValue returns the textual representation used by decoded pickle strings.
func stringValue(value any) (text string) {
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	}

	return
}
