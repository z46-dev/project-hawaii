package wowsreplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ResourceBundleSchemaVersion is the current imported-resource manifest format.
	ResourceBundleSchemaVersion = 1

	resourceBundleManifestName = "wowsreplay-manifest.json"
)

type (
	// ResourceBundleManifest identifies a version-matched imported game-data bundle.
	ResourceBundleManifest struct {
		SchemaVersion   int                 `json:"schemaVersion"`
		Version         string              `json:"version"`
		Build           uint32              `json:"build"`
		ImportedAt      time.Time           `json:"importedAt"`
		Extractor       string              `json:"extractor"`
		ExtractorSHA256 string              `json:"extractorSha256,omitempty"`
		VFSDirectory    string              `json:"vfsDirectory"`
		Stats           ResourceBundleStats `json:"stats"`
	}

	// ResourceBundleStats summarizes renderer and semantic resources in a bundle.
	ResourceBundleStats struct {
		EntityDefinitionFiles int `json:"entityDefinitionFiles"`
		Maps                  int `json:"maps"`
		MapFiles              int `json:"mapFiles"`
		ShipIconFiles         int `json:"shipIconFiles"`
		PlaneIconFiles        int `json:"planeIconFiles"`
		GUIFiles              int `json:"guiFiles"`
	}

	// ResourceBundle is a validated imported game-data directory.
	ResourceBundle struct {
		Root     string
		Manifest ResourceBundleManifest
	}
)

// OpenResourceBundle validates and opens one imported resource bundle.
func OpenResourceBundle(root string) (bundle *ResourceBundle, err error) {
	var (
		data     []byte
		manifest ResourceBundleManifest
		stats    ResourceBundleStats
	)

	if data, err = os.ReadFile(filepath.Join(root, resourceBundleManifestName)); err != nil {
		err = fmt.Errorf("read resource bundle manifest: %w", err)

		return
	}

	if err = json.Unmarshal(data, &manifest); err != nil {
		err = fmt.Errorf("decode resource bundle manifest: %w", err)

		return
	}

	if manifest.SchemaVersion != ResourceBundleSchemaVersion {
		err = fmt.Errorf(
			"unsupported resource bundle schema %d, expected %d",
			manifest.SchemaVersion,
			ResourceBundleSchemaVersion,
		)

		return
	}

	if stats, err = InspectResourceBundle(root); err != nil {
		return
	}

	manifest.Stats = stats
	bundle = &ResourceBundle{Root: root, Manifest: manifest}

	return
}

// FindResourceBundle locates and opens a bundle by exact WoWS build number.
func FindResourceBundle(baseDirectory string, build uint32) (bundle *ResourceBundle, err error) {
	var matches []string

	if matches, err = filepath.Glob(filepath.Join(baseDirectory, "*_"+strconv.FormatUint(uint64(build), 10))); err != nil {
		return
	}

	if len(matches) == 0 {
		err = os.ErrNotExist

		return
	}

	if len(matches) > 1 {
		err = fmt.Errorf("multiple resource bundles found for build %d", build)

		return
	}

	bundle, err = OpenResourceBundle(matches[0])

	return
}

// InspectResourceBundle verifies required replay resources and counts optional artwork.
func InspectResourceBundle(root string) (stats ResourceBundleStats, err error) {
	var (
		vfsRoot string = filepath.Join(root, "vfs")
	)

	for _, requiredPath := range []string{
		"content/GameParams.data",
		"scripts/entities.xml",
		"scripts/entity_defs/alias.xml",
	} {
		var info os.FileInfo

		if info, err = os.Stat(filepath.Join(vfsRoot, filepath.FromSlash(requiredPath))); err != nil {
			err = fmt.Errorf("resource bundle is missing %s: %w", requiredPath, err)

			return
		}

		if info.Size() == 0 {
			err = fmt.Errorf("resource bundle contains an empty %s", requiredPath)

			return
		}
	}

	if stats.EntityDefinitionFiles, err = countFiles(filepath.Join(vfsRoot, "scripts", "entity_defs"), nil); err != nil {
		return
	}

	if stats.GUIFiles, err = countFiles(filepath.Join(vfsRoot, "gui"), nil); err != nil {
		return
	}

	if stats.ShipIconFiles, err = countFiles(
		filepath.Join(vfsRoot, "gui", "fla", "minimap"),
		map[string]bool{".svg": true},
	); err != nil {
		return
	}

	if stats.PlaneIconFiles, err = countFiles(
		filepath.Join(vfsRoot, "gui", "battle_hud", "markers_minimap", "plane"),
		map[string]bool{".png": true},
	); err != nil {
		return
	}

	if stats.Maps, stats.MapFiles, err = countMaps(filepath.Join(vfsRoot, "spaces")); err != nil {
		return
	}

	if stats.EntityDefinitionFiles == 0 {
		err = errors.New("resource bundle contains no entity definitions")
	} else if stats.Maps == 0 {
		err = errors.New("resource bundle contains no minimap artwork")
	}

	return
}

// VFSPath resolves a slash-separated game VFS path within the bundle.
func (bundle *ResourceBundle) VFSPath(vfsPath string) (path string, err error) {
	var (
		cleaned string = filepath.Clean(filepath.FromSlash(vfsPath))
		root    string = filepath.Join(bundle.Root, bundle.Manifest.VFSDirectory)
	)

	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		err = fmt.Errorf("invalid VFS path %q", vfsPath)

		return
	}

	path = filepath.Join(root, cleaned)

	return
}

// WriteResourceBundleManifest writes a validated importer manifest into a bundle.
func WriteResourceBundleManifest(root string, manifest ResourceBundleManifest) (err error) {
	var data []byte

	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = ResourceBundleSchemaVersion
	}

	if manifest.VFSDirectory == "" {
		manifest.VFSDirectory = "vfs"
	}

	if manifest.Stats, err = InspectResourceBundle(root); err != nil {
		return
	}

	if data, err = json.MarshalIndent(manifest, "", "  "); err != nil {
		return
	}

	data = append(data, '\n')
	if err = os.WriteFile(filepath.Join(root, resourceBundleManifestName), data, 0o644); err != nil {
		err = fmt.Errorf("write resource bundle manifest: %w", err)
	}

	return
}

// countFiles recursively counts regular files, optionally filtering extensions.
func countFiles(root string, extensions map[string]bool) (count int, err error) {
	if _, err = os.Stat(root); errors.Is(err, os.ErrNotExist) {
		err = nil

		return
	} else if err != nil {
		return
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) (walkResult error) {
		if walkErr != nil {
			walkResult = walkErr

			return
		}

		if entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			if extensions == nil || extensions[strings.ToLower(filepath.Ext(entry.Name()))] {
				count++
			}
		}

		return
	})

	return
}

// countMaps counts map directories with minimap artwork and their imported files.
func countMaps(root string) (maps, files int, err error) {
	var entries []os.DirEntry

	if entries, err = os.ReadDir(root); errors.Is(err, os.ErrNotExist) {
		err = nil

		return
	} else if err != nil {
		return
	}

	for _, entry := range entries {
		var (
			hasMinimap bool
			mapRoot    string
		)

		if !entry.IsDir() {
			continue
		}

		mapRoot = filepath.Join(root, entry.Name())
		for _, name := range []string{"minimap.png", "minimap_water.png", "space.settings"} {
			var statErr error

			if _, statErr = os.Stat(filepath.Join(mapRoot, name)); statErr == nil {
				files++
				if name == "minimap.png" {
					hasMinimap = true
				}
			}
		}

		if hasMinimap {
			maps++
		}
	}

	return
}
