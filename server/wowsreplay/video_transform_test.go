package wowsreplay

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestImportedMapBounds supports both attribute and child-element settings formats.
func TestImportedMapBounds(t *testing.T) {
	var (
		bounds      mapBounds
		bundle      *ResourceBundle
		entries     []os.DirEntry
		err         error
		spacesPath  string
		fixtureRoot string = filepath.Join("testdata", "game", "15.6.0_12830008")
	)

	if bundle, err = OpenResourceBundle(fixtureRoot); err != nil {
		t.Skip("local 15.6 resource bundle is unavailable")
	}

	if spacesPath, err = bundle.VFSPath("spaces"); err != nil {
		t.Fatal(err)
	}
	if entries, err = os.ReadDir(spacesPath); err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "" || entry.Name()[0] < '0' || entry.Name()[0] > '9' {
			continue
		}

		var settingsPath string = filepath.Join(spacesPath, entry.Name(), "space.settings")

		if _, err = os.Stat(settingsPath); os.IsNotExist(err) {
			continue
		}
		if bounds, err = loadMapBounds(bundle, "spaces/"+entry.Name()); err != nil {
			t.Errorf("%s: %v", entry.Name(), err)
		}
	}

	if bounds, err = loadMapBounds(bundle, "spaces/19_OC_prey"); err != nil {
		t.Fatal(err)
	}
	if bounds.minX != -700 || bounds.maxX != 700 || bounds.minZ != -700 || bounds.maxZ != 700 {
		t.Fatalf("19_OC_prey bounds are mis-scaled: %#v", bounds)
	}
}

// TestShipIconYaw normalizes opposite-facing enemy source artwork.
func TestShipIconYaw(t *testing.T) {
	var match *Match = &Match{
		OwnShipID: 1,
		Entities: map[uint32]*MatchEntity{
			1: {ID: 1, TeamID: 0, TeamKnown: true},
			2: {ID: 2, TeamID: 1, TeamKnown: true},
		},
	}

	if math.Abs(float64(shipIconYaw(match, 1, 0)+math.Pi/2)) > 0.0001 {
		t.Fatalf("own icon rotation is incorrect: %f", shipIconYaw(match, 1, 0))
	}
	if math.Abs(float64(shipIconYaw(match, 2, 0)-math.Pi/2)) > 0.0001 {
		t.Fatalf("enemy icon rotation is incorrect: %f", shipIconYaw(match, 2, 0))
	}
}

// TestViewportBounds applies a centered mode crop without changing world registration.
func TestViewportBounds(t *testing.T) {
	var (
		base  mapBounds = mapBounds{minX: -700, maxX: 700, minZ: -700, maxZ: 700}
		match *Match    = &Match{
			Viewport: MatchViewport{
				Center: Vector3{X: 100, Z: -50},
				Width:  1000,
				Height: 800,
			},
		}
		viewport mapBounds
	)

	viewport = viewportBounds(match, base)
	if viewport.minX != -400 || viewport.maxX != 600 ||
		viewport.minZ != -450 || viewport.maxZ != 350 {
		t.Fatalf("unexpected battle viewport: %#v", viewport)
	}
}

// TestFriendlyPalette uses the green team identity from World of Warships.
func TestFriendlyPalette(t *testing.T) {
	var (
		friendly color.RGBA
		match    *Match = &Match{
			OwnShipID: 1,
			Entities: map[uint32]*MatchEntity{
				1: {ID: 1, TeamID: 0, TeamKnown: true},
				2: {ID: 2, TeamID: 0, TeamKnown: true},
			},
		}
	)

	friendly = teamColor(match, 0, 255)
	if friendly.G <= friendly.R || friendly.G <= friendly.B {
		t.Fatalf("friendly objective color is not green: %#v", friendly)
	}

	friendly = matchEntityColor(match, 2, false)
	if friendly.G <= friendly.R || friendly.G <= friendly.B {
		t.Fatalf("friendly entity color is not green: %#v", friendly)
	}
}

// TestTraceDisplayDuration preserves transmitted ammunition flight lifetimes.
func TestTraceDisplayDuration(t *testing.T) {
	var shellDuration float32 = traceDisplayDuration(WeaponTrace{Kind: "shell", Duration: 15})
	var torpedoDuration float32 = traceDisplayDuration(WeaponTrace{Kind: "torpedo", Duration: 15})

	if shellDuration != 15 || torpedoDuration != 15 {
		t.Fatalf("unexpected trace lifetimes: shell=%f torpedo=%f", shellDuration, torpedoDuration)
	}
}

// TestTorpedoTraceColors distinguishes own, allied, and enemy launchers.
func TestTorpedoTraceColors(t *testing.T) {
	var (
		match *Match = &Match{
			OwnShipID: 1,
			Entities: map[uint32]*MatchEntity{
				1: {ID: 1, TeamID: 0, TeamKnown: true},
				2: {ID: 2, TeamID: 0, TeamKnown: true},
				3: {ID: 3, TeamID: 1, TeamKnown: true},
			},
		}
		own   color.RGBA = torpedoTraceColor(match, WeaponTrace{OwnerID: 1})
		ally  color.RGBA = torpedoTraceColor(match, WeaponTrace{OwnerID: 2})
		enemy color.RGBA = torpedoTraceColor(match, WeaponTrace{OwnerID: 3})
	)

	if own.G <= own.R || own.G <= own.B {
		t.Fatalf("own torpedo is not green: %#v", own)
	}
	if ally.R != ally.G && ally.G != ally.B {
		t.Fatalf("allied torpedo is not neutral white: %#v", ally)
	}
	if enemy.R <= enemy.G || enemy.R <= enemy.B {
		t.Fatalf("enemy torpedo is not red: %#v", enemy)
	}
}

// TestInterpolatePositionTracks smooths movement and uses the shortest heading turn.
func TestInterpolatePositionTracks(t *testing.T) {
	var (
		cursors map[uint32]int              = make(map[uint32]int)
		tracks  map[uint32][]positionSample = map[uint32][]positionSample{
			7: {
				{
					clock:    0,
					entityID: 7,
					position: Vector3{X: 0},
					rotation: Rotation{Yaw: float32(170 * math.Pi / 180)},
				},
				{
					clock:    2,
					entityID: 7,
					position: Vector3{X: 10},
					rotation: Rotation{Yaw: float32(-170 * math.Pi / 180)},
				},
			},
		}
		current map[uint32]positionSample
	)

	current = interpolatePositionTracks(tracks, cursors, 1)
	if math.Abs(float64(current[7].position.X-5)) > 0.0001 {
		t.Fatalf("position was not interpolated: %f", current[7].position.X)
	}
	if math.Abs(math.Abs(float64(current[7].rotation.Yaw))-math.Pi) > 0.0001 {
		t.Fatalf("heading took the long rotational path: %f", current[7].rotation.Yaw)
	}
}

// TestInterpolatePositionTracksDoesNotBridgeGaps preserves last-known positions across long gaps.
func TestInterpolatePositionTracksDoesNotBridgeGaps(t *testing.T) {
	var (
		current map[uint32]positionSample
		tracks  map[uint32][]positionSample = map[uint32][]positionSample{
			4: {
				{clock: 0, entityID: 4, position: Vector3{X: 2}},
				{clock: 10, entityID: 4, position: Vector3{X: 20}},
			},
		}
	)

	current = interpolatePositionTracks(tracks, make(map[uint32]int), 5)
	if current[4].position.X != 2 {
		t.Fatalf("long hidden gap was interpolated: %f", current[4].position.X)
	}
}

// TestClipTorpedoTracesStopsAtTerrain prevents torpedoes from crossing islands.
func TestClipTorpedoTracesStopsAtTerrain(t *testing.T) {
	var (
		bounds  mapBounds     = mapBounds{minX: 0, maxX: 100, minZ: 0, maxZ: 100}
		terrain *image.RGBA   = image.NewRGBA(image.Rect(0, 0, 101, 101))
		traces  []WeaponTrace = []WeaponTrace{{
			Clock:    1,
			Duration: 8,
			Kind:     "torpedo",
			Start:    Vector3{X: 10, Z: 50},
			End:      Vector3{X: 90, Z: 50},
			Range:    80,
		}}
	)

	for y := 0; y < terrain.Bounds().Dy(); y++ {
		for x := 50; x < terrain.Bounds().Dx(); x++ {
			terrain.SetRGBA(x, y, color.RGBA{R: 80, G: 90, B: 70, A: 255})
		}
	}

	clipTorpedoTraces(traces, bounds, terrain)
	if traces[0].End.X >= 50 || traces[0].Duration >= 8 || traces[0].Range >= 80 {
		t.Fatalf("torpedo was not clipped before terrain: %#v", traces[0])
	}
}

// TestAmmunitionMarkersAreDistinct keeps torpedo triangles separate from shell dots.
func TestAmmunitionMarkersAreDistinct(t *testing.T) {
	var (
		bounds         mapBounds   = mapBounds{minX: 0, maxX: 100, minZ: 0, maxZ: 100}
		center         Vector3     = Vector3{X: 50, Z: 50}
		match          *Match      = &Match{OwnShipID: 1}
		secondaryFrame *image.RGBA = image.NewRGBA(image.Rect(0, 0, 101, 101))
		shellFrame     *image.RGBA = image.NewRGBA(image.Rect(0, 0, 101, 101))
		torpedoFrame   *image.RGBA = image.NewRGBA(image.Rect(0, 0, 101, 101))
		trace          WeaponTrace = WeaponTrace{
			Duration: 10,
			OwnerID:  1,
			Start:    Vector3{X: 10, Z: 50},
			End:      Vector3{X: 90, Z: 50},
		}
	)

	drawTorpedoTrace(torpedoFrame, trace, bounds, center, match)
	drawShellTrace(shellFrame, trace, bounds, center, 0.5)
	trace.Secondary = true
	drawShellTrace(secondaryFrame, trace, bounds, center, 0.5)

	var (
		secondaryPixels int         = visiblePixelCount(secondaryFrame)
		shellPixels     int         = visiblePixelCount(shellFrame)
		torpedoPixels   int         = visiblePixelCount(torpedoFrame)
		torpedoCenter   image.Point = mapPoint(center, bounds, torpedoFrame.Bounds())
	)

	if torpedoPixels <= shellPixels {
		t.Fatalf("torpedo triangle is not more distinct than shell dot: torpedo=%d shell=%d", torpedoPixels, shellPixels)
	}
	if secondaryPixels >= shellPixels {
		t.Fatalf("secondary shell is not smaller: secondary=%d main=%d", secondaryPixels, shellPixels)
	}
	if torpedoFrame.RGBAAt(torpedoCenter.X, torpedoCenter.Y+3).A == 0 {
		t.Fatal("torpedo does not have a downward triangle tip")
	}
}

// TestVideoTimingUsesExactMultiplier defines one as real-time and ten as ten-times speed.
func TestVideoTimingUsesExactMultiplier(t *testing.T) {
	var options VideoOptions = DefaultVideoOptions()

	if options.PlaybackSpeed != 4 {
		t.Fatalf("unexpected default playback multiplier: %f", options.PlaybackSpeed)
	}
	if options.Width != 960 || options.Height != 960 {
		t.Fatalf("unexpected default resolution: %dx%d", options.Width, options.Height)
	}

	options.FramesPerSecond = 24
	options.PlaybackSpeed = 10
	if replayClockAtFrame(240, options) != 100 {
		t.Fatalf("ten output seconds did not advance one hundred replay seconds: %f", replayClockAtFrame(240, options))
	}
	if videoFrameCount(600, options) != 1440 {
		t.Fatalf("ten-times rendering did not produce sixty output seconds: %d frames", videoFrameCount(600, options))
	}

	options.PlaybackSpeed = 1
	if videoFrameCount(600, options) != 14400 {
		t.Fatalf("real-time rendering did not preserve duration: %d frames", videoFrameCount(600, options))
	}
}

// TestSortLineupParticipants applies class priority before the requested nation priority.
func TestSortLineupParticipants(t *testing.T) {
	var participants []MatchParticipant = []MatchParticipant{
		{PlayerName: "dd", ShipClass: "Destroyer", Nation: "japan"},
		{PlayerName: "cv-fr", ShipClass: "AirCarrier", Nation: "france"},
		{PlayerName: "ca-us", ShipClass: "Cruiser", Nation: "usa"},
		{PlayerName: "ss", ShipClass: "Submarine", Nation: "germany"},
		{PlayerName: "bb", ShipClass: "Battleship", Nation: "germany"},
		{PlayerName: "bb-uk", ShipClass: "Battleship", Nation: "United_Kingdom"},
		{PlayerName: "bb-ru", ShipClass: "Battleship", Nation: "Russia"},
		{PlayerName: "ca-jp", ShipClass: "Cruiser", Nation: "japan"},
		{PlayerName: "cv-us", ShipClass: "AirCarrier", Nation: "usa"},
	}
	var expected []string = []string{
		"cv-us",
		"cv-fr",
		"bb-ru",
		"bb",
		"bb-uk",
		"ca-jp",
		"ca-us",
		"dd",
		"ss",
	}

	sortLineupParticipants(participants)
	for index := range expected {
		if participants[index].PlayerName != expected[index] {
			t.Fatalf("unexpected lineup position %d: got %s, want %s", index, participants[index].PlayerName, expected[index])
		}
	}
}

// visiblePixelCount counts rasterized pixels in a renderer test frame.
func visiblePixelCount(frame *image.RGBA) (count int) {
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			if frame.RGBAAt(x, y).A > 0 {
				count++
			}
		}
	}

	return
}
