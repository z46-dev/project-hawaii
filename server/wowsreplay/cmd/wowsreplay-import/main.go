// Command wowsreplay-import imports version-matched data from a WoWS installation.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/z46-dev/project-hawaii/wowsreplay"
	"github.com/z46-dev/project-hawaii/wowsreplay/internal/gameimport"
)

type (
	importOptions struct {
		installDirectory string
		outputDirectory  string
		build            uint64
		force            bool
	}
)

// main imports the current or selected game build.
func main() {
	var (
		err     error
		options importOptions
	)

	if options, err = parseFlags(); err == nil {
		err = run(options)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates importer command-line arguments.
func parseFlags() (options importOptions, err error) {
	flag.StringVar(&options.installDirectory, "install", "", "World of Warships installation directory")
	flag.StringVar(
		&options.outputDirectory,
		"output",
		"wowsreplay/testdata/game",
		"base directory for versioned resource bundles",
	)
	flag.Uint64Var(&options.build, "build", 0, "exact numeric WoWS build; defaults to newest installed build")
	flag.BoolVar(&options.force, "force", false, "replace an existing bundle for the selected build")
	flag.Parse()

	if options.installDirectory == "" && flag.NArg() == 1 {
		options.installDirectory = flag.Arg(0)
	}

	if options.installDirectory == "" {
		err = errors.New("provide the WoWS installation directory as -install <path> or the sole positional argument")
	} else if flag.NArg() > 1 {
		err = errors.New("only one positional installation directory is accepted")
	}

	return
}

// run detects the installed build, imports it, and validates the resource bundle.
func run(options importOptions) (err error) {
	var (
		build  uint32
		bundle *wowsreplay.ResourceBundle
		result gameimport.Result
	)

	if options.installDirectory, err = filepath.Abs(options.installDirectory); err != nil {
		return
	}

	if options.outputDirectory, err = filepath.Abs(options.outputDirectory); err != nil {
		return
	}

	if err = validateInstallDirectory(options.installDirectory); err != nil {
		return
	}

	if options.build == 0 {
		if build, err = detectNewestBuild(options.installDirectory); err != nil {
			return
		}
	} else if options.build > uint64(^uint32(0)) {
		err = fmt.Errorf("build %d exceeds uint32", options.build)

		return
	} else {
		build = uint32(options.build)
	}

	if !options.force {
		if bundle, err = wowsreplay.FindResourceBundle(options.outputDirectory, build); err == nil {
			printBundle(bundle)

			return
		} else if !errors.Is(err, os.ErrNotExist) {
			return
		}
	}

	fmt.Printf("Importing WoWS build %d from %s\n", build, options.installDirectory)
	if result, err = gameimport.Import(
		options.installDirectory,
		options.outputDirectory,
		build,
		options.force,
	); err != nil {
		return
	}

	if err = wowsreplay.WriteResourceBundleManifest(result.Root, wowsreplay.ResourceBundleManifest{
		Version:    result.Version,
		Build:      result.Build,
		ImportedAt: time.Now().UTC(),
		Extractor:  "wowsreplay native IDX/PKG importer",
	}); err != nil {
		return
	}

	if bundle, err = wowsreplay.OpenResourceBundle(result.Root); err != nil {
		return
	}

	fmt.Printf("Extracted %d files (%s)\n", result.FileCount, formatBytes(result.ByteCount))
	printBundle(bundle)

	return
}

// validateInstallDirectory checks for the archive structures needed by the importer.
func validateInstallDirectory(installDirectory string) (err error) {
	for _, requiredDirectory := range []string{"bin", "res_packages"} {
		var info os.FileInfo

		if info, err = os.Stat(filepath.Join(installDirectory, requiredDirectory)); err != nil {
			err = fmt.Errorf("invalid WoWS installation: missing %s: %w", requiredDirectory, err)

			return
		}

		if !info.IsDir() {
			err = fmt.Errorf("invalid WoWS installation: %s is not a directory", requiredDirectory)

			return
		}
	}

	return
}

// detectNewestBuild returns the largest numeric directory under bin.
func detectNewestBuild(installDirectory string) (build uint32, err error) {
	var entries []os.DirEntry

	if entries, err = os.ReadDir(filepath.Join(installDirectory, "bin")); err != nil {
		return
	}

	for _, entry := range entries {
		var candidate uint64

		if !entry.IsDir() {
			continue
		}

		if candidate, err = strconv.ParseUint(entry.Name(), 10, 32); err != nil {
			err = nil

			continue
		}

		if uint32(candidate) > build {
			build = uint32(candidate)
		}
	}

	if build == 0 {
		err = errors.New("no numeric WoWS build directories found")
	}

	return
}

// printBundle reports the resources available to replay renderers.
func printBundle(bundle *wowsreplay.ResourceBundle) {
	fmt.Printf("Resource bundle: %s\n", bundle.Root)
	fmt.Printf(
		"Version %s, %d maps, %d map files, %d entity definitions, %d ship icons, %d plane icons, %d GUI files\n",
		bundle.Manifest.Version,
		bundle.Manifest.Stats.Maps,
		bundle.Manifest.Stats.MapFiles,
		bundle.Manifest.Stats.EntityDefinitionFiles,
		bundle.Manifest.Stats.ShipIconFiles,
		bundle.Manifest.Stats.PlaneIconFiles,
		bundle.Manifest.Stats.GUIFiles,
	)
}

// formatBytes returns a compact binary byte count for progress output.
func formatBytes(count int64) (formatted string) {
	if count >= 1<<30 {
		formatted = fmt.Sprintf("%.1f GiB", float64(count)/float64(1<<30))
	} else if count >= 1<<20 {
		formatted = fmt.Sprintf("%.1f MiB", float64(count)/float64(1<<20))
	} else if count >= 1<<10 {
		formatted = fmt.Sprintf("%.1f KiB", float64(count)/float64(1<<10))
	} else {
		formatted = fmt.Sprintf("%d B", count)
	}

	return
}
