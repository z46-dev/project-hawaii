package wowsreplay

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSemanticDecoderWithLocalReplay validates the imported schema against a real fixture when present.
func TestSemanticDecoderWithLocalReplay(t *testing.T) {
	var (
		bundle       *ResourceBundle
		decodeErrors int
		decoded      int
		entries      []os.DirEntry
		err          error
		replay       *Replay
	)

	if bundle, err = OpenResourceBundle(filepath.Join("testdata", "game", "15.6.0_12830008")); err != nil {
		t.Skip("local 15.6 resource bundle is unavailable")
	}

	if entries, err = os.ReadDir(filepath.Join("testdata", "replays")); err != nil || len(entries) == 0 {
		t.Skip("local replay fixtures are unavailable")
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".wowsreplay" {
			if replay, err = ParseFileWithResources(
				filepath.Join("testdata", "replays", entry.Name()),
				bundle,
			); err != nil {
				t.Fatal(err)
			}

			break
		}
	}

	if replay == nil {
		t.Skip("local replay fixtures are unavailable")
	}

	for _, event := range replay.Events {
		switch data := event.Data.(type) {
		case SemanticMethodData:
			decoded++
			if data.DecodeError != "" {
				decodeErrors++
			}
		case SemanticPropertyData:
			decoded++
			if data.DecodeError != "" {
				decodeErrors++
			}
		}
	}

	if decoded == 0 {
		t.Fatal("semantic decoder handled no entity events")
	}

	var match *Match = replay.BuildMatch()

	if len(match.Entities) == 0 {
		t.Fatal("match reconstruction produced no entities")
	}

	if len(match.Traces) == 0 {
		t.Fatal("match reconstruction produced no weapon traces")
	}

	if len(match.Entities[match.OwnShipID].Positions) < 100 {
		t.Fatalf("own ship movement was not reconstructed: %d samples", len(match.Entities[match.OwnShipID].Positions))
	}

	var (
		enemyPositions int
		objectives     int
		ships          map[uint64]ShipParameters
	)

	if ships, err = bundle.LoadShipParameters(); err != nil {
		t.Fatal(err)
	}
	match.EnrichShipParameters(ships)

	for _, entity := range match.Entities {
		if entity.Type == "Vehicle" && entity.TeamKnown && entity.TeamID != match.Entities[match.OwnShipID].TeamID {
			enemyPositions += len(entity.Positions)
		}
		if entity.Objective != nil {
			objectives++
		}
		if entity.Type == "Vehicle" && entity.ShipID != 0 && entity.ShipClass == "" {
			t.Fatalf("ship %d has no GameParams class", entity.ShipID)
		}
	}

	if enemyPositions == 0 {
		t.Fatal("enemy movement was not reconstructed")
	}
	if objectives == 0 {
		t.Fatal("capture objectives were not reconstructed")
	}

	t.Logf(
		"decoded %d method/property events with %d diagnostics; reconstructed %d entities and %d traces",
		decoded,
		decodeErrors,
		len(match.Entities),
		len(match.Traces),
	)

	if _, err = exec.LookPath("ffmpeg"); err == nil {
		var options VideoOptions = DefaultVideoOptions()

		options.Width = 256
		options.Height = 256
		options.FramesPerSecond = 2
		options.PlaybackSpeed = 100
		options.Resources = bundle
		if err = replay.RenderVideo(
			context.Background(),
			filepath.Join(t.TempDir(), "semantic.mp4"),
			options,
		); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSemanticDecoderWithAllLocalReplays checks schema coverage across every supplied fixture.
func TestSemanticDecoderWithAllLocalReplays(t *testing.T) {
	var (
		bundle  *ResourceBundle
		borders map[uint64]MapBorderParameters
		entries []os.DirEntry
		err     error
		ships   map[uint64]ShipParameters
	)

	if bundle, err = OpenResourceBundle(filepath.Join("testdata", "game", "15.6.0_12830008")); err != nil {
		t.Skip("local 15.6 resource bundle is unavailable")
	}

	if entries, err = os.ReadDir(filepath.Join("testdata", "replays")); err != nil {
		t.Skip("local replay fixtures are unavailable")
	}
	if ships, err = bundle.LoadShipParameters(); err != nil {
		t.Fatal(err)
	}
	if borders, err = bundle.LoadMapBorderParameters(); err != nil {
		t.Fatal(err)
	}

	var replayCount int

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".wowsreplay" {
			continue
		}
		replayCount++

		var replay *Replay

		if replay, err = ParseFileWithResources(
			filepath.Join("testdata", "replays", entry.Name()),
			bundle,
		); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}

		for _, event := range replay.Events {
			switch data := event.Data.(type) {
			case SemanticMethodData:
				if data.DecodeError != "" {
					t.Fatalf("%s event %d: %s", entry.Name(), event.Index, data.DecodeError)
				}
			case SemanticPropertyData:
				if data.DecodeError != "" {
					t.Fatalf("%s event %d: %s", entry.Name(), event.Index, data.DecodeError)
				}
			case SemanticNestedPropertyData:
				if data.DecodeError != "" {
					t.Fatalf("%s event %d: %s", entry.Name(), event.Index, data.DecodeError)
				}
			}
		}

		var match *Match = replay.BuildMatch()
		var (
			found   bool
			ownShip *MatchEntity
		)

		match.EnrichShipParameters(ships)
		match.EnrichMapBorders(borders)
		if ownShip, found = match.Entities[match.OwnShipID]; !found || len(ownShip.Positions) < 2 {
			t.Fatalf("%s: recording ship has no movement track", entry.Name())
		}
		if match.Viewport.Width <= 0 || match.Viewport.Height <= 0 {
			t.Fatalf("%s: map-border viewport was not resolved", entry.Name())
		}

		for _, entity := range match.Entities {
			if entity.Type == "Vehicle" && entity.ShipID != 0 && entity.ShipClass == "" {
				t.Fatalf("%s: ship %d has no class", entry.Name(), entity.ShipID)
			}
		}
	}

	if replayCount == 0 {
		t.Skip("local replay fixtures are unavailable")
	}
}
