// Command wowsreplay-export batch-exports replay event logs and tactical videos.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z46-dev/project-hawaii/wowsreplay"
)

type (
	exportOptions struct {
		inputDirectory    string
		outputDirectory   string
		includePayload    bool
		renderVideo       bool
		renderMatch       bool
		resourceDirectory string
		videoOptions      wowsreplay.VideoOptions
	}

	exportRecord struct {
		Replay         string   `json:"replay"`
		Version        string   `json:"version,omitempty"`
		Map            string   `json:"map,omitempty"`
		Player         string   `json:"player,omitempty"`
		Events         int      `json:"events,omitempty"`
		Positions      int      `json:"positions,omitempty"`
		Log            string   `json:"log,omitempty"`
		LogBytes       int64    `json:"logBytes,omitempty"`
		Match          string   `json:"match,omitempty"`
		MatchBytes     int64    `json:"matchBytes,omitempty"`
		Video          string   `json:"video,omitempty"`
		VideoBytes     int64    `json:"videoBytes,omitempty"`
		Warnings       []string `json:"warnings,omitempty"`
		Error          string   `json:"error,omitempty"`
		ElapsedSeconds float64  `json:"elapsedSeconds"`
	}

	exportManifest struct {
		GeneratedAt time.Time      `json:"generatedAt"`
		Input       string         `json:"input"`
		Output      string         `json:"output"`
		Exports     []exportRecord `json:"exports"`
	}
)

// main parses flags and exports every replay in the input directory.
func main() {
	var (
		err     error
		options exportOptions = parseFlags()
	)

	if err = run(context.Background(), options); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseFlags builds batch-export options from command-line arguments.
func parseFlags() (options exportOptions) {
	options.videoOptions = wowsreplay.DefaultVideoOptions()

	flag.StringVar(
		&options.inputDirectory,
		"input",
		"wowsreplay/testdata/replays",
		".wowsreplay file or directory containing replay files",
	)
	flag.StringVar(&options.outputDirectory, "output", "wowsreplay/testdata/exports", "directory for exported artifacts")
	flag.BoolVar(&options.includePayload, "raw-payload", true, "include base64 raw packet payloads in JSONL logs")
	flag.BoolVar(&options.renderVideo, "video", true, "render tactical MP4 videos")
	flag.BoolVar(&options.renderMatch, "match", true, "write reconstructed semantic match JSON when resources are set")
	flag.StringVar(&options.resourceDirectory, "resources", "", "exact imported <version>_<build> resource bundle")
	flag.IntVar(&options.videoOptions.Width, "video-width", options.videoOptions.Width, "video width in pixels")
	flag.IntVar(&options.videoOptions.Height, "video-height", options.videoOptions.Height, "video height in pixels")
	flag.IntVar(
		&options.videoOptions.FramesPerSecond,
		"video-fps",
		options.videoOptions.FramesPerSecond,
		"video frames per second",
	)
	flag.Float64Var(
		&options.videoOptions.PlaybackSpeed,
		"video-speed",
		options.videoOptions.PlaybackSpeed,
		"playback multiplier where 1 is real-time and 10 is ten-times speed",
	)
	flag.IntVar(&options.videoOptions.CRF, "video-crf", options.videoOptions.CRF, "H.264 CRF quality from 1 to 51")
	flag.Parse()

	return
}

// run discovers, exports, and records every replay without stopping after one bad fixture.
func run(ctx context.Context, options exportOptions) (err error) {
	var (
		failures int
		manifest exportManifest = exportManifest{
			GeneratedAt: time.Now().UTC(),
			Input:       options.inputDirectory,
			Output:      options.outputDirectory,
		}
		matches []string
	)

	var inputInfo os.FileInfo

	if inputInfo, err = os.Stat(options.inputDirectory); err != nil {
		return
	}

	if inputInfo.IsDir() {
		if matches, err = filepath.Glob(filepath.Join(options.inputDirectory, "*.wowsreplay")); err != nil {
			return
		}
	} else if strings.EqualFold(filepath.Ext(options.inputDirectory), ".wowsreplay") {
		matches = []string{options.inputDirectory}
	}

	if len(matches) == 0 {
		err = fmt.Errorf("no .wowsreplay files found in %q", options.inputDirectory)

		return
	}

	if err = os.MkdirAll(options.outputDirectory, 0o755); err != nil {
		err = fmt.Errorf("create output directory: %w", err)

		return
	}

	if options.resourceDirectory != "" {
		if options.videoOptions.Resources, err = wowsreplay.OpenResourceBundle(options.resourceDirectory); err != nil {
			err = fmt.Errorf("open resources: %w", err)

			return
		}
	}

	for _, replayPath := range matches {
		var record exportRecord

		record = exportReplay(ctx, replayPath, options)
		manifest.Exports = append(manifest.Exports, record)

		if record.Error != "" {
			failures++
			fmt.Fprintf(os.Stderr, "FAILED %s: %s\n", record.Replay, record.Error)
		} else {
			fmt.Printf(
				"EXPORTED %s: %d events, %.1f MiB log, %.1f MiB video, %.1fs\n",
				record.Replay,
				record.Events,
				float64(record.LogBytes)/(1<<20),
				float64(record.VideoBytes)/(1<<20),
				record.ElapsedSeconds,
			)
		}
	}

	if err = writeManifest(filepath.Join(options.outputDirectory, "manifest.json"), manifest); err != nil {
		return
	}

	if failures > 0 {
		err = fmt.Errorf("%d of %d replay exports failed; see manifest.json", failures, len(matches))
	}

	return
}

// exportReplay writes all requested artifacts for one replay.
func exportReplay(ctx context.Context, replayPath string, options exportOptions) (record exportRecord) {
	var (
		baseName string = strings.TrimSuffix(filepath.Base(replayPath), filepath.Ext(replayPath))
		closeErr error
		err      error
		logFile  *os.File
		replay   *wowsreplay.Replay
		started  time.Time = time.Now()
	)

	record.Replay = filepath.Base(replayPath)
	if replay, err = wowsreplay.ParseFile(replayPath); err != nil {
		record.Error = err.Error()
		record.ElapsedSeconds = time.Since(started).Seconds()

		return
	}

	record.Version = replay.Metadata.ClientVersionFromExe
	record.Map = replay.Metadata.MapDisplayName
	record.Player = replay.Metadata.PlayerName
	record.Events = len(replay.Events)
	record.Positions = countPositions(replay.Events)
	record.Warnings = replay.Warnings
	record.Log = baseName + ".events.jsonl"

	if options.videoOptions.Resources != nil {
		if err = replay.DecodeWithResources(options.videoOptions.Resources); err != nil {
			record.Error = fmt.Sprintf("decode semantic events: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}
	}

	if logFile, err = os.Create(filepath.Join(options.outputDirectory, record.Log)); err != nil {
		record.Error = fmt.Sprintf("create event log: %v", err)
		record.ElapsedSeconds = time.Since(started).Seconds()

		return
	}

	if err = replay.WriteEventLog(logFile, wowsreplay.LogOptions{
		Format:         wowsreplay.LogFormatJSONLines,
		IncludePayload: options.includePayload,
	}); err != nil {
		record.Error = fmt.Sprintf("write event log: %v", err)
	}

	if closeErr = logFile.Close(); err == nil && closeErr != nil {
		err = closeErr
		record.Error = fmt.Sprintf("close event log: %v", closeErr)
	}

	if err != nil {
		record.ElapsedSeconds = time.Since(started).Seconds()

		return
	}

	record.LogBytes = fileSize(filepath.Join(options.outputDirectory, record.Log))

	if options.renderMatch && options.videoOptions.Resources != nil {
		var (
			borders     map[uint64]wowsreplay.MapBorderParameters
			match       *wowsreplay.Match = replay.BuildMatch()
			projectiles map[uint64]wowsreplay.ProjectileParameters
			ships       map[uint64]wowsreplay.ShipParameters
		)

		if ships, err = options.videoOptions.Resources.LoadShipParameters(); err != nil {
			record.Error = fmt.Sprintf("load ship parameters: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}
		match.EnrichShipParameters(ships)
		if borders, err = options.videoOptions.Resources.LoadMapBorderParameters(); err != nil {
			record.Error = fmt.Sprintf("load map-border parameters: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}
		match.EnrichMapBorders(borders)
		if projectiles, err = options.videoOptions.Resources.LoadProjectileParameters(); err != nil {
			record.Error = fmt.Sprintf("load projectile parameters: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}
		match.EnrichProjectileParameters(projectiles)
		options.videoOptions.Match = match
		record.Match = baseName + ".match.json"
		if err = writeJSON(
			filepath.Join(options.outputDirectory, record.Match),
			match,
		); err != nil {
			record.Error = fmt.Sprintf("write match reconstruction: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}
		record.MatchBytes = fileSize(filepath.Join(options.outputDirectory, record.Match))
	}

	if options.renderVideo {
		record.Video = baseName + ".mp4"
		if err = replay.RenderVideo(ctx, filepath.Join(options.outputDirectory, record.Video), options.videoOptions); err != nil {
			record.Error = fmt.Sprintf("render video: %v", err)
			record.ElapsedSeconds = time.Since(started).Seconds()

			return
		}

		record.VideoBytes = fileSize(filepath.Join(options.outputDirectory, record.Video))
	}

	record.ElapsedSeconds = time.Since(started).Seconds()

	return
}

// countPositions counts typed volatile and initial position events.
func countPositions(events []wowsreplay.Event) (count int) {
	for _, event := range events {
		switch event.Data.(type) {
		case wowsreplay.PositionData,
			wowsreplay.EntityCreateData,
			wowsreplay.CellPlayerCreateData,
			wowsreplay.SemanticEntityCreateData,
			wowsreplay.SemanticCellPlayerCreateData:
			count++
		}
	}

	return
}

// fileSize returns an artifact size or zero when stat fails.
func fileSize(path string) (size int64) {
	var (
		err  error
		info os.FileInfo
	)

	if info, err = os.Stat(path); err == nil {
		size = info.Size()
	}

	return
}

// writeManifest replaces the batch manifest after all exports finish.
func writeManifest(path string, manifest exportManifest) (err error) {
	err = writeJSON(path, manifest)

	return
}

// writeJSON writes one indented JSON artifact.
func writeJSON(path string, value any) (err error) {
	var (
		data []byte
	)

	if data, err = json.MarshalIndent(value, "", "  "); err != nil {
		return
	}

	data = append(data, '\n')
	err = os.WriteFile(path, data, 0o644)

	return
}
