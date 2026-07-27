// Package gameimport extracts the replay resources stored in WoWS IDX/PKG archives.
package gameimport

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	idxMagic       uint32 = 0x50465349
	idxVersionV40  uint32 = 0x02000000
	rootParentID   uint64 = 0xdbb1a1d1b108b927
	maxParentDepth        = 256
)

var versionPattern *regexp.Regexp = regexp.MustCompile(
	`curVersion_(?i:release_)?([0-9]+)_([0-9]+)_([0-9]+)(?:_([0-9]+))?_([0-9]+)`,
)

type (
	// Result describes the resource directory produced by Import.
	Result struct {
		Root      string
		Version   string
		Build     uint32
		FileCount int
		ByteCount int64
	}

	resource struct {
		name     string
		id       uint64
		parentID uint64
	}

	fileInfo struct {
		resourceID       uint64
		volumeID         uint64
		offset           uint64
		compression      uint64
		size             uint32
		crc32            uint32
		uncompressedSize uint32
	}

	archiveEntry struct {
		path     string
		volume   string
		fileInfo fileInfo
	}

	index struct {
		resources map[uint64]resource
		files     map[uint64]fileInfo
		volumes   map[uint64]string
	}
)

// Import reads an installed build and writes the resources needed by the replay package.
func Import(installDirectory, outputDirectory string, build uint32, force bool) (result Result, err error) {
	var (
		accountEntry archiveEntry
		entries      []archiveEntry
		version      string
	)

	if entries, err = loadEntries(filepath.Join(
		installDirectory,
		"bin",
		strconv.FormatUint(uint64(build), 10),
		"idx",
	)); err != nil {
		return
	}

	if accountEntry, err = findEntry(entries, "scripts/entity_defs/Account.def"); err != nil {
		return
	}

	if version, err = detectVersion(
		filepath.Join(installDirectory, "res_packages"),
		accountEntry,
		build,
	); err != nil {
		return
	}

	result = Result{
		Root:    filepath.Join(outputDirectory, version+"_"+strconv.FormatUint(uint64(build), 10)),
		Version: version,
		Build:   build,
	}

	if _, err = os.Stat(result.Root); err == nil {
		if !force {
			err = fmt.Errorf("resource bundle already exists: %s", result.Root)

			return
		}

		if err = os.RemoveAll(result.Root); err != nil {
			err = fmt.Errorf("replace resource bundle: %w", err)

			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return
	}

	err = nil
	if err = extractSelected(
		filepath.Join(installDirectory, "res_packages"),
		filepath.Join(result.Root, "vfs"),
		entries,
		&result,
	); err != nil {
		return
	}

	if err = copyTranslations(installDirectory, result.Root, build); err != nil {
		return
	}

	return
}

// loadEntries parses and combines every modern IDX file for a build.
func loadEntries(indexDirectory string) (entries []archiveEntry, err error) {
	var (
		combined index = index{
			resources: make(map[uint64]resource),
			files:     make(map[uint64]fileInfo),
			volumes:   make(map[uint64]string),
		}
		directoryEntries []os.DirEntry
	)

	if directoryEntries, err = os.ReadDir(indexDirectory); err != nil {
		err = fmt.Errorf("read IDX directory: %w", err)

		return
	}

	for _, directoryEntry := range directoryEntries {
		var parsed index

		if directoryEntry.IsDir() || !strings.EqualFold(filepath.Ext(directoryEntry.Name()), ".idx") {
			continue
		}

		if parsed, err = parseIDX(filepath.Join(indexDirectory, directoryEntry.Name())); err != nil {
			err = fmt.Errorf("parse %s: %w", directoryEntry.Name(), err)

			return
		}

		for id, item := range parsed.resources {
			combined.resources[id] = item
		}

		for id, item := range parsed.files {
			combined.files[id] = item
		}

		for id, name := range parsed.volumes {
			combined.volumes[id] = name
		}
	}

	for id, item := range combined.files {
		var (
			path       string
			volumeName string
		)

		if volumeName = combined.volumes[item.volumeID]; volumeName == "" {
			continue
		}

		if path, err = resolvePath(id, combined.resources, make(map[uint64]bool), 0); err != nil {
			return
		}

		entries = append(entries, archiveEntry{
			path:     strings.TrimPrefix(filepath.ToSlash(path), "/"),
			volume:   volumeName,
			fileInfo: item,
		})
	}

	return
}

// parseIDX reads the modern 64-bit IDX format used by current WoWS builds.
func parseIDX(path string) (parsed index, err error) {
	var data []byte

	if data, err = os.ReadFile(path); err != nil {
		return
	}

	if len(data) < 56 || binary.LittleEndian.Uint32(data[0:4]) != idxMagic {
		err = errors.New("invalid IDX header")

		return
	}

	if binary.LittleEndian.Uint32(data[4:8]) != idxVersionV40 {
		err = fmt.Errorf("unsupported IDX version 0x%08x", binary.LittleEndian.Uint32(data[4:8]))

		return
	}

	parsed = index{
		resources: make(map[uint64]resource),
		files:     make(map[uint64]fileInfo),
		volumes:   make(map[uint64]string),
	}

	var (
		resourceCount uint64 = uint64(binary.LittleEndian.Uint32(data[16:20]))
		fileCount     uint64 = uint64(binary.LittleEndian.Uint32(data[20:24]))
		volumeCount   uint64 = uint64(binary.LittleEndian.Uint32(data[24:28]))
		resourceBase  uint64 = 16 + binary.LittleEndian.Uint64(data[32:40])
		fileBase      uint64 = 16 + binary.LittleEndian.Uint64(data[40:48])
		volumeBase    uint64 = 16 + binary.LittleEndian.Uint64(data[48:56])
	)

	if err = checkTable(data, resourceBase, resourceCount, 32); err != nil {
		return
	}

	if err = checkTable(data, fileBase, fileCount, 48); err != nil {
		return
	}

	if err = checkTable(data, volumeBase, volumeCount, 24); err != nil {
		return
	}

	for number := uint64(0); number < resourceCount; number++ {
		var (
			offset     uint64 = resourceBase + number*32
			nameOffset uint64 = offset + binary.LittleEndian.Uint64(data[offset+8:offset+16])
			item       resource
		)

		if item.name, err = nullTerminated(data, nameOffset); err != nil {
			return
		}

		item.id = binary.LittleEndian.Uint64(data[offset+16 : offset+24])
		item.parentID = binary.LittleEndian.Uint64(data[offset+24 : offset+32])
		parsed.resources[item.id] = item
	}

	for number := uint64(0); number < fileCount; number++ {
		var (
			offset uint64 = fileBase + number*48
			item   fileInfo
		)

		item.resourceID = binary.LittleEndian.Uint64(data[offset : offset+8])
		item.volumeID = binary.LittleEndian.Uint64(data[offset+8 : offset+16])
		item.offset = binary.LittleEndian.Uint64(data[offset+16 : offset+24])
		item.compression = binary.LittleEndian.Uint64(data[offset+24 : offset+32])
		item.size = binary.LittleEndian.Uint32(data[offset+32 : offset+36])
		item.crc32 = binary.LittleEndian.Uint32(data[offset+36 : offset+40])
		item.uncompressedSize = binary.LittleEndian.Uint32(data[offset+40 : offset+44])
		parsed.files[item.resourceID] = item
	}

	for number := uint64(0); number < volumeCount; number++ {
		var (
			offset     uint64 = volumeBase + number*24
			nameOffset uint64 = offset + binary.LittleEndian.Uint64(data[offset+8:offset+16])
			name       string
			volumeID   uint64 = binary.LittleEndian.Uint64(data[offset+16 : offset+24])
		)

		if name, err = nullTerminated(data, nameOffset); err != nil {
			return
		}

		parsed.volumes[volumeID] = strings.TrimPrefix(name, "//.//")
	}

	return
}

// extractSelected copies only semantic, map, and renderer resources into plain files.
func extractSelected(packageDirectory, vfsDirectory string, entries []archiveEntry, result *Result) (err error) {
	var selected int

	for _, entry := range entries {
		if !wantedPath(entry.path) {
			continue
		}

		if err = extractEntry(packageDirectory, vfsDirectory, entry); err != nil {
			err = fmt.Errorf("extract %s: %w", entry.path, err)

			return
		}

		selected++
		result.FileCount++
		result.ByteCount += int64(entry.fileInfo.uncompressedSize)
		if selected%250 == 0 {
			fmt.Printf("Extracted %d resource files...\n", selected)
		}
	}

	if selected == 0 {
		err = errors.New("no matching replay resources were found in the installed archives")
	}

	return
}

// extractEntry reads, inflates, verifies, and writes one archive member.
func extractEntry(packageDirectory, vfsDirectory string, entry archiveEntry) (err error) {
	var (
		compressed []byte = make([]byte, entry.fileInfo.size)
		data       []byte
		file       *os.File
		reader     io.ReadCloser
	)

	if file, err = os.Open(filepath.Join(packageDirectory, filepath.Base(entry.volume))); err != nil {
		return
	}

	if _, err = file.ReadAt(compressed, int64(entry.fileInfo.offset)); err != nil {
		file.Close()

		return
	}

	if err = file.Close(); err != nil {
		return
	}

	if entry.fileInfo.compression == 0 {
		data = compressed
	} else {
		reader = flate.NewReader(bytes.NewReader(compressed))
		if data, err = io.ReadAll(reader); err != nil {
			reader.Close()

			return
		}

		if err = reader.Close(); err != nil {
			return
		}
	}

	if uint32(len(data)) != entry.fileInfo.uncompressedSize {
		err = fmt.Errorf(
			"size mismatch: got %d bytes, expected %d",
			len(data),
			entry.fileInfo.uncompressedSize,
		)

		return
	}

	if crc32.ChecksumIEEE(data) != entry.fileInfo.crc32 {
		err = errors.New("CRC32 mismatch")

		return
	}

	var destination string

	if destination, err = safeDestination(vfsDirectory, entry.path); err != nil {
		return
	}

	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return
	}

	err = os.WriteFile(destination, data, 0o644)

	return
}

// detectVersion reads the Account.def version marker directly from the packages.
func detectVersion(packageDirectory string, entry archiveEntry, expectedBuild uint32) (version string, err error) {
	var (
		data    []byte
		matches [][]byte
	)

	if data, err = readEntry(packageDirectory, entry); err != nil {
		return
	}

	if matches = versionPattern.FindSubmatch(data); len(matches) == 0 {
		err = errors.New("could not find curVersion marker in Account.def")

		return
	}

	var (
		major uint64
		minor uint64
		patch uint64
		build uint64
	)

	if major, err = strconv.ParseUint(string(matches[1]), 10, 32); err != nil {
		return
	}

	if minor, err = strconv.ParseUint(string(matches[2]), 10, 32); err != nil {
		return
	}

	if patch, err = strconv.ParseUint(string(matches[3]), 10, 32); err != nil {
		return
	}

	if build, err = strconv.ParseUint(string(matches[len(matches)-1]), 10, 32); err != nil {
		return
	}

	if uint32(build) != expectedBuild {
		err = fmt.Errorf("Account.def reports build %d, expected %d", build, expectedBuild)

		return
	}

	if major >= 1 && major <= 11 {
		patch = minor
		minor = major
		major = 0
	}

	version = fmt.Sprintf("%d.%d.%d", major, minor, patch)

	return
}

// readEntry returns one archive member without writing it.
func readEntry(packageDirectory string, entry archiveEntry) (data []byte, err error) {
	var temporaryDirectory string

	if temporaryDirectory, err = os.MkdirTemp("", "wowsreplay-import-version-*"); err != nil {
		return
	}
	defer os.RemoveAll(temporaryDirectory)

	if err = extractEntry(packageDirectory, temporaryDirectory, entry); err != nil {
		return
	}

	data, err = os.ReadFile(filepath.Join(temporaryDirectory, filepath.FromSlash(entry.path)))

	return
}

// wantedPath reports whether a VFS member belongs in a replay resource bundle.
func wantedPath(path string) (wanted bool) {
	var normalized string = strings.ToLower(strings.TrimPrefix(filepath.ToSlash(path), "/"))

	for _, prefix := range []string{
		"gui/fla/minimap/",
		"gui/battle_hud/",
		"gui/consumables/",
		"gui/powerups/drops/",
		"gui/fonts/",
		"gui/data/constants/",
		"gui/ships_silhouettes/",
		"gui/ribbons/",
		"gui/achievements/",
		"gui/nation_flags/",
		"gui/crew_commander/skills/",
		"gui/modernization_icons/",
		"gui/signal_flags/",
		"scripts/entity_defs/",
	} {
		if strings.HasPrefix(normalized, prefix) {
			wanted = true

			return
		}
	}

	if normalized == "content/gameparams.data" || normalized == "scripts/entities.xml" {
		wanted = true

		return
	}

	var parts []string = strings.Split(normalized, "/")

	if len(parts) == 3 && parts[0] == "spaces" {
		wanted = parts[2] == "minimap.png" ||
			parts[2] == "minimap_water.png" ||
			parts[2] == "space.settings"
	} else if len(parts) == 4 && parts[0] == "content" && parts[1] == "gameplay" {
		wanted = parts[3] == "space.settings"
	}

	return
}

// resolvePath reconstructs a VFS path from the IDX parent chain.
func resolvePath(
	id uint64,
	resources map[uint64]resource,
	visiting map[uint64]bool,
	depth int,
) (path string, err error) {
	var (
		found bool
		item  resource
	)

	if depth > maxParentDepth || visiting[id] {
		err = fmt.Errorf("invalid resource parent chain at %x", id)

		return
	}

	if item, found = resources[id]; !found {
		err = fmt.Errorf("missing resource %x", id)

		return
	}

	if item.parentID == rootParentID {
		path = item.name

		return
	}

	visiting[id] = true
	if path, err = resolvePath(item.parentID, resources, visiting, depth+1); err == nil {
		path += "/" + item.name
	}
	delete(visiting, id)

	return
}

// findEntry locates one VFS path case-insensitively.
func findEntry(entries []archiveEntry, path string) (found archiveEntry, err error) {
	for _, entry := range entries {
		if strings.EqualFold(entry.path, path) {
			found = entry

			return
		}
	}

	err = fmt.Errorf("installed archives do not contain %s", path)

	return
}

// checkTable ensures a fixed-width IDX table is in bounds.
func checkTable(data []byte, offset, count, width uint64) (err error) {
	if offset > uint64(len(data)) || count > (uint64(len(data))-offset)/width {
		err = errors.New("IDX table is out of bounds")
	}

	return
}

// nullTerminated reads an IDX string with a conservative bounds check.
func nullTerminated(data []byte, offset uint64) (value string, err error) {
	if offset >= uint64(len(data)) {
		err = errors.New("IDX string offset is out of bounds")

		return
	}

	var end int = int(offset)

	for end < len(data) && data[end] != 0 {
		end++
	}

	if end == len(data) {
		err = errors.New("unterminated IDX string")
	} else {
		value = string(data[int(offset):end])
	}

	return
}

// safeDestination prevents archive paths from escaping the bundle VFS directory.
func safeDestination(root, archivePath string) (destination string, err error) {
	var clean string = filepath.Clean(filepath.FromSlash(strings.TrimPrefix(archivePath, "/")))

	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		err = fmt.Errorf("unsafe archive path %q", archivePath)

		return
	}

	destination = filepath.Join(root, clean)

	return
}

// copyTranslations copies the installed locale catalogs when available.
func copyTranslations(installDirectory, bundleRoot string, build uint32) (err error) {
	var (
		sourceRoot string = filepath.Join(
			installDirectory,
			"bin",
			strconv.FormatUint(uint64(build), 10),
			"res",
			"texts",
		)
		entries []os.DirEntry
	)

	if entries, err = os.ReadDir(sourceRoot); errors.Is(err, os.ErrNotExist) {
		err = nil

		return
	} else if err != nil {
		return
	}

	for _, entry := range entries {
		var (
			data        []byte
			source      string
			destination string
		)

		if !entry.IsDir() {
			continue
		}

		source = filepath.Join(sourceRoot, entry.Name(), "LC_MESSAGES", "global.mo")
		if data, err = os.ReadFile(source); errors.Is(err, os.ErrNotExist) {
			err = nil

			continue
		} else if err != nil {
			return
		}

		destination = filepath.Join(bundleRoot, "translations", entry.Name(), "LC_MESSAGES", "global.mo")
		if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return
		}

		if err = os.WriteFile(destination, data, 0o644); err != nil {
			return
		}
	}

	return
}
