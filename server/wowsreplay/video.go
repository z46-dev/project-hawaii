package wowsreplay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"encoding/xml"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	// ErrFFmpegNotFound indicates that video output was requested without an ffmpeg executable.
	ErrFFmpegNotFound = errors.New("ffmpeg executable not found")

	// ErrNoPositionData indicates that a replay has no renderable position updates.
	ErrNoPositionData = errors.New("replay contains no renderable position data")
)

type (
	// VideoOptions controls the compact tactical-map MP4 renderer.
	VideoOptions struct {
		Width           int
		Height          int
		FramesPerSecond int
		PlaybackSpeed   float64
		CRF             int
		Preset          string
		TrailSeconds    float32
		MaxEntities     int
		FFmpegPath      string
		Resources       *ResourceBundle
		Match           *Match
	}

	positionSample struct {
		clock    float32
		entityID uint32
		position Vector3
		rotation Rotation
	}

	entityTrack struct {
		entityID uint32
		count    int
	}

	mapBounds struct {
		minX float32
		maxX float32
		minZ float32
		maxZ float32
	}

	renderState struct {
		current map[uint32]positionSample
		history map[uint32][]positionSample
		status  map[uint32]renderEntityStatus
	}

	renderEntityStatus struct {
		health           float64
		maxHealth        float64
		alive            bool
		visible          bool
		teamID           int32
		objectiveRadius  float64
		objectiveIndex   int
		objectiveType    int
		objectiveEnabled bool
		objectiveVisible bool
		captureProgress  float64
		hasInvaders      bool
		invaderTeam      int32
	}

	renderStateChange struct {
		clock    float32
		entityID uint32
		property string
		value    any
	}

	videoAssets struct {
		background  *image.RGBA
		icons       map[string]*image.RGBA
		regularFace font.Face
		boldFace    font.Face
		scale       float64
		textCache   map[textCacheKey]cachedVideoText
		fitCache    map[textFitKey]string
	}

	textCacheKey struct {
		text  string
		color color.RGBA
		bold  bool
	}

	textFitKey struct {
		text  string
		width int
		bold  bool
	}

	cachedVideoText struct {
		image  *image.RGBA
		ascent int
	}
)

// DefaultVideoOptions returns a smooth, high-compression tactical video profile.
func DefaultVideoOptions() (options VideoOptions) {
	options = VideoOptions{
		Width:           960,
		Height:          960,
		FramesPerSecond: 24,
		PlaybackSpeed:   4,
		CRF:             30,
		Preset:          "medium",
		TrailSeconds:    45,
		MaxEntities:     64,
		FFmpegPath:      "ffmpeg",
	}

	return
}

// RenderVideo creates a compact MP4 tactical-map video at path.
func (replay *Replay) RenderVideo(ctx context.Context, path string, options VideoOptions) (err error) {
	var file *os.File

	if file, err = os.Create(path); err != nil {
		err = fmt.Errorf("create video %q: %w", path, err)

		return
	}

	if err = replay.WriteVideo(ctx, file, options); err != nil {
		file.Close()
		err = fmt.Errorf("render video %q: %w", path, err)

		return
	}

	if err = file.Close(); err != nil {
		err = fmt.Errorf("close video %q: %w", path, err)
	}

	return
}

// WriteVideo streams a fragmented MP4 tactical-map rendering to writer.
func (replay *Replay) WriteVideo(ctx context.Context, writer io.Writer, options VideoOptions) (err error) {
	var (
		activeTraces   []WeaponTrace
		assets         videoAssets
		bounds         mapBounds
		borders        map[uint64]MapBorderParameters
		changeIndex    int
		changes        []renderStateChange
		command        *exec.Cmd
		commandError   bytes.Buffer
		closeErr       error
		eventIndex     int
		ffmpegInput    io.WriteCloser
		frame          *image.RGBA
		frameCount     int
		match          *Match
		ownShipID      uint32
		projectiles    map[uint64]ProjectileParameters
		renderDuration float32
		samples        []positionSample
		selected       map[uint32]bool
		ships          map[uint64]ShipParameters
		state          renderState
		traceIndex     int
		trackCursors   map[uint32]int
		tracks         map[uint32][]positionSample
		waitErr        error
	)

	if options, err = normalizeVideoOptions(options); err != nil {
		return
	}

	if options.Resources != nil && options.Match == nil {
		if err = replay.DecodeWithResources(options.Resources); err != nil {
			return
		}
	}

	if options.Match != nil {
		match = options.Match
	} else {
		match = replay.BuildMatch()
	}
	if options.Resources != nil {
		if ships, err = options.Resources.LoadShipParameters(); err != nil {
			return
		}
		match.EnrichShipParameters(ships)
		if borders, err = options.Resources.LoadMapBorderParameters(); err != nil {
			return
		}
		match.EnrichMapBorders(borders)
		if projectiles, err = options.Resources.LoadProjectileParameters(); err != nil {
			return
		}
		match.EnrichProjectileParameters(projectiles)
	}
	options.Match = match
	samples, ownShipID = match.positionSamples()
	if len(samples) == 0 {
		err = ErrNoPositionData

		return
	}

	selected = selectEntities(samples, ownShipID, options.MaxEntities, match)
	samples = filterSamples(samples, selected)
	tracks = positionTracks(samples)
	trackCursors = make(map[uint32]int, len(tracks))
	bounds = boundsForSamples(samples)
	assets.scale = videoInterfaceScale(options)
	if options.Resources != nil {
		if assets, bounds, err = prepareVideoAssets(replay, options, bounds); err != nil {
			return
		}
		clipTorpedoTraces(match.Traces, bounds, assets.background)
	}
	if err = assets.prepareFonts(options.Resources); err != nil {
		return
	}
	defer assets.close()

	changes = renderChanges(match, selected)
	renderDuration = max(match.Duration, samples[len(samples)-1].clock)
	frameCount = videoFrameCount(renderDuration, options)

	command = exec.CommandContext(
		ctx,
		options.FFmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "rawvideo",
		"-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", options.Width, options.Height),
		"-framerate", fmt.Sprintf("%d", options.FramesPerSecond),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", options.Preset,
		"-crf", fmt.Sprintf("%d", options.CRF),
		"-tune", "animation",
		"-pix_fmt", "yuv420p",
		"-movflags", "frag_keyframe+empty_moov+faststart",
		"-f", "mp4",
		"pipe:1",
	)
	command.Stdout = writer
	command.Stderr = &commandError

	if ffmpegInput, err = command.StdinPipe(); err != nil {
		return
	}

	if err = command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			err = fmt.Errorf("%w: %s", ErrFFmpegNotFound, options.FFmpegPath)
		}

		return
	}

	state = renderState{
		current: make(map[uint32]positionSample),
		history: make(map[uint32][]positionSample),
		status:  make(map[uint32]renderEntityStatus),
	}
	for entityID := range selected {
		state.status[entityID] = renderEntityStatus{alive: true, visible: true}
	}
	frame = image.NewRGBA(image.Rect(0, 0, options.Width, options.Height))

	for frameIndex := 0; frameIndex < frameCount; frameIndex++ {
		var replayClock float32 = replayClockAtFrame(frameIndex, options)

		for eventIndex < len(samples) && samples[eventIndex].clock <= replayClock {
			var sample positionSample = samples[eventIndex]

			state.history[sample.entityID] = append(state.history[sample.entityID], sample)
			eventIndex++
		}
		state.current = interpolatePositionTracks(tracks, trackCursors, replayClock)

		for changeIndex < len(changes) && changes[changeIndex].clock <= replayClock {
			applyRenderChange(state.status, changes[changeIndex])
			changeIndex++
		}

		for traceIndex < len(match.Traces) && match.Traces[traceIndex].Clock <= replayClock {
			activeTraces = append(activeTraces, match.Traces[traceIndex])
			traceIndex++
		}
		activeTraces = retainActiveTraces(activeTraces, replayClock)

		drawVideoFrame(
			frame,
			state,
			bounds,
			replayClock,
			renderDuration,
			ownShipID,
			options,
			match,
			assets,
			activeTraces,
		)

		if _, err = ffmpegInput.Write(frame.Pix); err != nil {
			break
		}
	}

	if closeErr = ffmpegInput.Close(); err == nil {
		err = closeErr
	}

	if waitErr = command.Wait(); err == nil && waitErr != nil {
		err = fmt.Errorf("ffmpeg: %w: %s", waitErr, commandError.String())
	}

	return
}

// videoFrameCount converts replay duration into output frames at the selected speed.
func videoFrameCount(duration float32, options VideoOptions) (count int) {
	count = int(math.Ceil(
		float64(duration) /
			options.PlaybackSpeed *
			float64(options.FramesPerSecond),
	))
	count = max(count, 1)

	return
}

// replayClockAtFrame maps output time to replay time using an exact playback multiplier.
func replayClockAtFrame(frameIndex int, options VideoOptions) (clock float32) {
	clock = float32(
		float64(frameIndex) /
			float64(options.FramesPerSecond) *
			options.PlaybackSpeed,
	)

	return
}

// positionSamples flattens reconstructed entity tracks for rendering.
func (match *Match) positionSamples() (samples []positionSample, ownShipID uint32) {
	ownShipID = match.OwnShipID

	for entityID, entity := range match.Entities {
		for _, position := range entity.Positions {
			samples = append(samples, positionSample{
				clock:    position.Clock,
				entityID: entityID,
				position: position.Position,
				rotation: position.Rotation,
			})
		}
	}

	sort.SliceStable(samples, func(left, right int) bool {
		return samples[left].clock < samples[right].clock
	})

	return
}

// positionTracks groups chronological position samples for frame interpolation.
func positionTracks(samples []positionSample) (tracks map[uint32][]positionSample) {
	tracks = make(map[uint32][]positionSample)

	for _, sample := range samples {
		tracks[sample.entityID] = append(tracks[sample.entityID], sample)
	}

	return
}

// interpolatePositionTracks samples smooth positions without bridging long visibility gaps.
func interpolatePositionTracks(
	tracks map[uint32][]positionSample,
	cursors map[uint32]int,
	clock float32,
) (current map[uint32]positionSample) {
	const maximumInterpolationGap float32 = 5

	current = make(map[uint32]positionSample, len(tracks))

	for entityID, track := range tracks {
		if len(track) == 0 || clock < track[0].clock {
			continue
		}

		var cursor int = cursors[entityID]

		for cursor+1 < len(track) && track[cursor+1].clock <= clock {
			cursor++
		}
		cursors[entityID] = cursor

		var sample positionSample = track[cursor]

		if cursor+1 < len(track) {
			var (
				next     positionSample = track[cursor+1]
				duration float32        = next.clock - sample.clock
			)

			if duration > 0 && duration <= maximumInterpolationGap {
				var progress float32 = min(max((clock-sample.clock)/duration, 0), 1)

				sample.position.X += (next.position.X - sample.position.X) * progress
				sample.position.Y += (next.position.Y - sample.position.Y) * progress
				sample.position.Z += (next.position.Z - sample.position.Z) * progress
				sample.rotation.Pitch += (next.rotation.Pitch - sample.rotation.Pitch) * progress
				sample.rotation.Roll += (next.rotation.Roll - sample.rotation.Roll) * progress
				sample.rotation.Yaw = interpolateAngle(sample.rotation.Yaw, next.rotation.Yaw, progress)
				sample.clock = clock
			}
		}
		current[entityID] = sample
	}

	return
}

// interpolateAngle follows the shortest rotational path between two headings.
func interpolateAngle(from, to, progress float32) (angle float32) {
	var delta float64 = math.Atan2(
		math.Sin(float64(to-from)),
		math.Cos(float64(to-from)),
	)

	angle = from + float32(delta)*progress

	return
}

// normalizeVideoOptions fills zero values and validates encoder settings.
func normalizeVideoOptions(options VideoOptions) (normalized VideoOptions, err error) {
	var defaults VideoOptions = DefaultVideoOptions()

	normalized = options
	if normalized.Width == 0 {
		normalized.Width = defaults.Width
	}

	if normalized.Height == 0 {
		normalized.Height = defaults.Height
	}

	if normalized.FramesPerSecond == 0 {
		normalized.FramesPerSecond = defaults.FramesPerSecond
	}

	if normalized.PlaybackSpeed == 0 {
		normalized.PlaybackSpeed = defaults.PlaybackSpeed
	}

	if normalized.CRF == 0 {
		normalized.CRF = defaults.CRF
	}

	if normalized.Preset == "" {
		normalized.Preset = defaults.Preset
	}

	if normalized.TrailSeconds == 0 {
		normalized.TrailSeconds = defaults.TrailSeconds
	}

	if normalized.MaxEntities == 0 {
		normalized.MaxEntities = defaults.MaxEntities
	}

	if normalized.FFmpegPath == "" {
		normalized.FFmpegPath = defaults.FFmpegPath
	}

	if normalized.Width < 64 || normalized.Height < 64 || normalized.Width%2 != 0 || normalized.Height%2 != 0 {
		err = errors.New("video width and height must be even and at least 64")
	} else if normalized.FramesPerSecond < 1 || normalized.FramesPerSecond > 60 {
		err = errors.New("video frames per second must be between 1 and 60")
	} else if normalized.PlaybackSpeed <= 0 {
		err = errors.New("video playback speed must be positive")
	} else if normalized.CRF < 1 || normalized.CRF > 51 {
		err = errors.New("video CRF must be between 1 and 51")
	} else if normalized.MaxEntities < 1 {
		err = errors.New("video maximum entity count must be positive")
	}

	return
}

// selectEntities keeps the longest-lived moving entities to avoid projectile clutter.
func selectEntities(
	samples []positionSample,
	ownShipID uint32,
	maximum int,
	match *Match,
) (selected map[uint32]bool) {
	var (
		counts map[uint32]int = make(map[uint32]int)
		tracks []entityTrack
	)

	for _, sample := range samples {
		counts[sample.entityID]++
	}

	tracks = make([]entityTrack, 0, len(counts))
	for entityID, count := range counts {
		if match != nil {
			if entity, found := match.Entities[entityID]; found && entity.Type != "" && entity.Type != "Vehicle" {
				continue
			}
		}

		tracks = append(tracks, entityTrack{entityID: entityID, count: count})
	}

	sort.Slice(tracks, func(left, right int) bool {
		return tracks[left].count > tracks[right].count
	})

	selected = make(map[uint32]bool, maximum)
	for index := 0; index < len(tracks) && index < maximum; index++ {
		selected[tracks[index].entityID] = true
	}

	if ownShipID != 0 {
		selected[ownShipID] = true
	}

	return
}

// filterSamples returns only position samples belonging to selected entities.
func filterSamples(samples []positionSample, selected map[uint32]bool) (filtered []positionSample) {
	filtered = make([]positionSample, 0, len(samples))

	for _, sample := range samples {
		if selected[sample.entityID] {
			filtered = append(filtered, sample)
		}
	}

	return
}

// boundsForSamples finds padded square map bounds from selected entity movement.
func boundsForSamples(samples []positionSample) (bounds mapBounds) {
	bounds = mapBounds{
		minX: samples[0].position.X,
		maxX: samples[0].position.X,
		minZ: samples[0].position.Z,
		maxZ: samples[0].position.Z,
	}

	for _, sample := range samples[1:] {
		bounds.minX = min(bounds.minX, sample.position.X)
		bounds.maxX = max(bounds.maxX, sample.position.X)
		bounds.minZ = min(bounds.minZ, sample.position.Z)
		bounds.maxZ = max(bounds.maxZ, sample.position.Z)
	}

	var (
		centerX float32 = (bounds.minX + bounds.maxX) / 2
		centerZ float32 = (bounds.minZ + bounds.maxZ) / 2
		span    float32 = max(bounds.maxX-bounds.minX, bounds.maxZ-bounds.minZ)
	)

	if span < 1000 {
		span = 1000
	}

	span *= 1.1
	bounds.minX = centerX - span/2
	bounds.maxX = centerX + span/2
	bounds.minZ = centerZ - span/2
	bounds.maxZ = centerZ + span/2

	return
}

// drawVideoFrame renders one intentionally minimal tactical-map frame.
func drawVideoFrame(
	frame *image.RGBA,
	state renderState,
	bounds mapBounds,
	clock,
	duration float32,
	ownShipID uint32,
	options VideoOptions,
	match *Match,
	assets videoAssets,
	traces []WeaponTrace,
) {
	draw.Draw(frame, frame.Bounds(), &image.Uniform{C: color.RGBA{R: 9, G: 22, B: 31, A: 255}}, image.Point{}, draw.Src)
	drawGrid(frame)
	if assets.background != nil {
		draw.Draw(frame, frame.Bounds(), assets.background, image.Point{}, draw.Over)
	}
	drawObjectiveMarkers(frame, match, state.status, bounds, assets)

	for _, trace := range traces {
		drawWeaponTrace(frame, trace, bounds, clock, match)
	}

	for entityID, history := range state.history {
		var entityColor color.RGBA = matchEntityColor(match, entityID, entityID == ownShipID)

		for index := len(history) - 1; index > 0; index-- {
			if history[index].clock < clock-options.TrailSeconds {
				break
			}

			var (
				from image.Point = mapPoint(history[index-1].position, bounds, frame.Bounds())
				to   image.Point = mapPoint(history[index].position, bounds, frame.Bounds())
			)

			drawLine(frame, from, to, color.RGBA{R: entityColor.R, G: entityColor.G, B: entityColor.B, A: 120})
		}
	}

	for entityID, sample := range state.current {
		var (
			entityColor color.RGBA = matchEntityColor(match, entityID, entityID == ownShipID)
			icon        *image.RGBA
			point       image.Point        = mapPoint(sample.position, bounds, frame.Bounds())
			radius      int                = 4
			status      renderEntityStatus = state.status[entityID]
		)

		if entityID == ownShipID {
			radius = 7
		}

		var opacity uint8 = 255

		if !status.alive {
			opacity = 210
		} else if !status.visible {
			opacity = 85
		}

		if icon = iconForEntity(assets, match, entityID, !status.alive); icon != nil {
			drawRotatedIcon(frame, icon, point, shipIconYaw(match, entityID, sample.rotation.Yaw), opacity)
		} else {
			if !status.visible {
				entityColor.A = opacity
			}
			drawHeadingMarker(frame, point, radius, sample.rotation.Yaw, entityColor)
		}

		if status.visible {
			drawHealthBar(frame, point, status, entityColor)
		}
		drawShipName(frame, assets, match, entityID, point, icon, opacity)
	}

	drawTeamLineups(frame, match, state.status, assets)

	var progress int

	if duration > 0 {
		progress = int(float32(frame.Bounds().Dx()-16) * min(clock/duration, 1))
	}

	draw.Draw(
		frame,
		image.Rect(8, frame.Bounds().Dy()-12, 8+progress, frame.Bounds().Dy()-6),
		&image.Uniform{C: color.RGBA{R: 230, G: 238, B: 242, A: 255}},
		image.Point{},
		draw.Src,
	)
}

// drawShipName places the catalog ship name beneath its tactical icon.
func drawShipName(
	frame *image.RGBA,
	assets videoAssets,
	match *Match,
	entityID uint32,
	point image.Point,
	icon *image.RGBA,
	opacity uint8,
) {
	if match == nil || assets.regularFace == nil {
		return
	}

	var (
		entity *MatchEntity
		found  bool
		label  string
		offset int = assets.pixels(18)
	)

	if entity, found = match.Entities[entityID]; !found {
		return
	}
	label = entity.ShipName
	if label == "" {
		label = entity.ShipClass
	}
	if label == "" {
		return
	}
	if icon != nil {
		offset = icon.Bounds().Dy()/2 + assets.pixels(11)
	}
	label = fitVideoText(assets, false, label, assets.pixels(110))

	drawCenteredText(
		frame,
		assets,
		false,
		point.X+1,
		point.Y+offset+assets.pixels(1),
		label,
		color.RGBA{R: 2, G: 8, B: 11, A: min(opacity, 190)},
	)
	drawCenteredText(
		frame,
		assets,
		false,
		point.X,
		point.Y+offset,
		label,
		color.RGBA{R: 232, G: 238, B: 240, A: opacity},
	)
}

// prepareVideoAssets loads and scales the real minimap and compact ship icons.
func prepareVideoAssets(
	replay *Replay,
	options VideoOptions,
	fallbackBounds mapBounds,
) (assets videoAssets, bounds mapBounds, err error) {
	bounds = fallbackBounds
	assets.icons = make(map[string]*image.RGBA)
	assets.scale = videoInterfaceScale(options)

	var mapPath string = strings.Trim(strings.ReplaceAll(replay.Metadata.MapName, "\\", "/"), "/")

	if mapPath == "" {
		mapPath = "spaces/" + strings.Trim(replay.Metadata.MapDisplayName, "/")
	}

	var backgroundPath string

	if backgroundPath, err = options.Resources.VFSPath(mapPath + "/minimap.png"); err != nil {
		return
	}

	var (
		backgroundFile  *os.File
		backgroundImage image.Image
		baseBounds      mapBounds
	)

	if backgroundFile, err = os.Open(backgroundPath); err != nil {
		err = fmt.Errorf("open minimap: %w", err)

		return
	}

	if backgroundImage, _, err = image.Decode(backgroundFile); err != nil {
		backgroundFile.Close()
		err = fmt.Errorf("decode minimap: %w", err)

		return
	}

	if err = backgroundFile.Close(); err != nil {
		return
	}
	if baseBounds, err = loadMapBounds(options.Resources, mapPath); err != nil {
		err = nil
		bounds = fallbackBounds
		assets.background = scaleImage(backgroundImage, options.Width, options.Height)
	} else {
		bounds = viewportBounds(options.Match, baseBounds)
		backgroundImage = cropMapImage(backgroundImage, baseBounds, bounds)
		assets.background = scaleImage(backgroundImage, options.Width, options.Height)
	}

	for _, species := range []string{"aircarrier", "auxiliary", "battleship", "cruiser", "destroyer", "submarine"} {
		for relation, prefix := range map[uint32]string{0: "", 1: "ally_", 2: "enemy_"} {
			for _, sunk := range []bool{false, true} {
				var (
					iconPath string
					name     string
				)

				if sunk {
					if relation == 2 {
						name = "icon_enemy_sunk_" + species + ".png"
					} else {
						name = "icon_sunk_" + species + ".png"
					}
				} else {
					name = "icon_" + prefix + species + ".png"
				}

				if iconPath, err = options.Resources.VFSPath(filepath.ToSlash(filepath.Join(
					"gui", "battle_hud", "markers", "ship", name,
				))); err != nil {
					return
				}

				if assets.icons[shipIconKey(relation, species, sunk)], err = loadScaledPNG(
					iconPath,
					assets.pixels(24),
					assets.pixels(24),
				); err != nil {
					err = nil
				}
			}
		}
	}

	return
}

// videoInterfaceScale keeps overlays proportional across output resolutions.
func videoInterfaceScale(options VideoOptions) (scale float64) {
	scale = float64(min(options.Width, options.Height)) / 720
	scale = min(max(scale, 0.75), 2)

	return
}

// pixels scales one baseline overlay dimension to the selected resolution.
func (assets videoAssets) pixels(value int) (pixels int) {
	var scale float64 = assets.scale

	if scale <= 0 {
		scale = 1
	}
	pixels = max(int(math.Round(float64(value)*scale)), 1)

	return
}

// prepareFonts loads the game's UI font and retains clean embedded faces as a standalone fallback.
func (assets *videoAssets) prepareFonts(bundle *ResourceBundle) (err error) {
	var (
		boldData    []byte = gobold.TTF
		regularData []byte = goregular.TTF
		path        string
		scale       float64 = assets.scale
	)

	if scale <= 0 {
		scale = 1
	}
	assets.textCache = make(map[textCacheKey]cachedVideoText)
	assets.fitCache = make(map[textFitKey]string)

	if bundle != nil {
		if path, err = bundle.VFSPath("gui/fonts/Warhelios_Regular.ttf"); err == nil {
			if regularData, err = os.ReadFile(path); err != nil {
				return
			}
		} else {
			err = nil
		}

		if path, err = bundle.VFSPath("gui/fonts/Warhelios_Bold.ttf"); err == nil {
			if boldData, err = os.ReadFile(path); err != nil {
				return
			}
		} else {
			err = nil
		}
	}

	if assets.regularFace, err = newVideoFontFace(regularData, 9*scale); err != nil {
		return
	}
	if assets.boldFace, err = newVideoFontFace(boldData, 10*scale); err != nil {
		return
	}

	return
}

// newVideoFontFace creates one hinted OpenType face for tactical-map labels.
func newVideoFontFace(data []byte, size float64) (face font.Face, err error) {
	var parsed *opentype.Font

	if parsed, err = opentype.Parse(data); err != nil {
		return
	}
	face, err = opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     96,
		Hinting: font.HintingFull,
	})

	return
}

// close releases OpenType face resources after rendering completes.
func (assets *videoAssets) close() {
	for _, face := range []font.Face{assets.regularFace, assets.boldFace} {
		if closer, found := face.(interface{ Close() error }); found {
			_ = closer.Close()
		}
	}
}

// viewportBounds applies the battle's GameParams map-border crop when available.
func viewportBounds(match *Match, base mapBounds) (viewport mapBounds) {
	viewport = base
	if match == nil || match.Viewport.Width <= 0 || match.Viewport.Height <= 0 {
		return
	}

	if match.Viewport.Width <= base.maxX-base.minX {
		viewport.minX = match.Viewport.Center.X - match.Viewport.Width/2
		viewport.maxX = match.Viewport.Center.X + match.Viewport.Width/2
	}
	if match.Viewport.Height <= base.maxZ-base.minZ {
		viewport.minZ = match.Viewport.Center.Z - match.Viewport.Height/2
		viewport.maxZ = match.Viewport.Center.Z + match.Viewport.Height/2
	}

	return
}

// cropMapImage selects the pixels corresponding to a playable world viewport.
func cropMapImage(source image.Image, base, viewport mapBounds) (cropped image.Image) {
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}

	var (
		sourceBounds image.Rectangle = source.Bounds()
		left         int             = sourceBounds.Min.X + int(
			(viewport.minX-base.minX)/(base.maxX-base.minX)*float32(sourceBounds.Dx()),
		)
		right int = sourceBounds.Min.X + int(
			(viewport.maxX-base.minX)/(base.maxX-base.minX)*float32(sourceBounds.Dx()),
		)
		top int = sourceBounds.Min.Y + int(
			(base.maxZ-viewport.maxZ)/(base.maxZ-base.minZ)*float32(sourceBounds.Dy()),
		)
		bottom int = sourceBounds.Min.Y + int(
			(base.maxZ-viewport.minZ)/(base.maxZ-base.minZ)*float32(sourceBounds.Dy()),
		)
		cropSource subImager
		found      bool
	)

	left = max(left, sourceBounds.Min.X)
	right = min(right, sourceBounds.Max.X)
	top = max(top, sourceBounds.Min.Y)
	bottom = min(bottom, sourceBounds.Max.Y)
	if cropSource, found = source.(subImager); right <= left || bottom <= top || !found {
		cropped = source
	} else {
		cropped = cropSource.SubImage(image.Rect(left, top, right, bottom))
	}

	return
}

// loadMapBounds converts WoWS chunk bounds into the replay's hundred-unit coordinates.
func loadMapBounds(bundle *ResourceBundle, mapPath string) (bounds mapBounds, err error) {
	type settingsDocument struct {
		Bounds struct {
			MinXAttribute float32 `xml:"minX,attr"`
			MaxXAttribute float32 `xml:"maxX,attr"`
			MinYAttribute float32 `xml:"minY,attr"`
			MaxYAttribute float32 `xml:"maxY,attr"`
			MinXElement   float32 `xml:"minX"`
			MaxXElement   float32 `xml:"maxX"`
			MinYElement   float32 `xml:"minY"`
			MaxYElement   float32 `xml:"maxY"`
		} `xml:"bounds"`
		ChunkSize float32 `xml:"chunkSize"`
		Terrain   struct {
			ChunkSize float32 `xml:"chunkSize"`
		} `xml:"terrain"`
	}

	var (
		document settingsDocument
		file     *os.File
		path     string
	)

	if path, err = bundle.VFSPath(mapPath + "/space.settings"); err != nil {
		return
	}

	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	if err = xml.NewDecoder(file).Decode(&document); err != nil {
		return
	}

	var chunkSize float32 = document.ChunkSize

	if chunkSize == 0 {
		chunkSize = document.Terrain.ChunkSize
	}
	if chunkSize == 0 {
		chunkSize = 100
	}

	var (
		minX float32 = document.Bounds.MinXAttribute
		maxX float32 = document.Bounds.MaxXAttribute
		minZ float32 = document.Bounds.MinYAttribute
		maxZ float32 = document.Bounds.MaxYAttribute
	)

	if maxX <= minX || maxZ <= minZ {
		minX = document.Bounds.MinXElement
		maxX = document.Bounds.MaxXElement
		minZ = document.Bounds.MinYElement
		maxZ = document.Bounds.MaxYElement
	}

	var (
		sizeX float32 = (maxX - minX - 3) * chunkSize
		sizeZ float32 = (maxZ - minZ - 3) * chunkSize
		size  float32 = max(sizeX, sizeZ)
	)

	bounds = mapBounds{
		minX: -size / 2,
		maxX: size / 2,
		minZ: -size / 2,
		maxZ: size / 2,
	}

	if bounds.maxX <= bounds.minX || bounds.maxZ <= bounds.minZ {
		err = errors.New("map settings contain invalid bounds")
	}

	return
}

// scaleImage performs a one-time nearest-neighbor resize for low-overhead frame rendering.
func scaleImage(source image.Image, width, height int) (scaled *image.RGBA) {
	scaled = image.NewRGBA(image.Rect(0, 0, width, height))

	var sourceBounds image.Rectangle = source.Bounds()

	for y := 0; y < height; y++ {
		var sourceY int = sourceBounds.Min.Y + y*sourceBounds.Dy()/height

		for x := 0; x < width; x++ {
			var sourceX int = sourceBounds.Min.X + x*sourceBounds.Dx()/width

			scaled.Set(x, y, source.At(sourceX, sourceY))
		}
	}

	return
}

// loadScaledPNG decodes and resizes one imported tactical icon.
func loadScaledPNG(path string, width, height int) (icon *image.RGBA, err error) {
	var (
		file   *os.File
		source image.Image
	)

	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	if source, _, err = image.Decode(file); err != nil {
		return
	}
	icon = scaleImage(source, width, height)

	return
}

// renderChanges flattens selected entity state changes into chronological order.
func renderChanges(match *Match, selected map[uint32]bool) (changes []renderStateChange) {
	for entityID, entity := range match.Entities {
		if !selected[entityID] && entity.Objective == nil {
			continue
		}

		for _, change := range entity.Changes {
			changes = append(changes, renderStateChange{
				clock: change.Clock, entityID: entityID, property: change.Property, value: change.Value,
			})
		}
	}

	sort.SliceStable(changes, func(left, right int) bool {
		return changes[left].clock < changes[right].clock
	})

	return
}

// applyRenderChange advances health, life, and visibility for one entity.
func applyRenderChange(statuses map[uint32]renderEntityStatus, change renderStateChange) {
	var status renderEntityStatus = statuses[change.entityID]

	switch change.property {
	case "health":
		status.health = numericValue(change.value)
	case "maxHealth":
		status.maxHealth = numericValue(change.value)
	case "isAlive":
		status.alive = numericValue(change.value) != 0
	case "__alive":
		status.alive = booleanValue(change.value)
	case "isInvisible":
		status.visible = numericValue(change.value) == 0
	case "teamId":
		status.teamID = int32(numericValue(change.value))
	case "__visible":
		status.visible, _ = change.value.(bool)
	case "__objective.radius":
		status.objectiveRadius = numericValue(change.value)
	case "__objective.index":
		status.objectiveIndex = int(numericValue(change.value))
	case "__objective.type":
		status.objectiveType = int(numericValue(change.value))
	case "__objective.isEnabled":
		status.objectiveEnabled = booleanValue(change.value)
	case "__objective.isVisible":
		status.objectiveVisible = booleanValue(change.value)
	case "__objective.progress":
		status.captureProgress = numericValue(change.value)
	case "__objective.hasInvaders":
		status.hasInvaders = booleanValue(change.value)
	case "__objective.invaderTeam":
		status.invaderTeam = int32(numericValue(change.value))
	}
	statuses[change.entityID] = status
}

// drawObjectiveMarkers renders capture-zone scale, ownership, labels, and progress.
func drawObjectiveMarkers(
	frame *image.RGBA,
	match *Match,
	statuses map[uint32]renderEntityStatus,
	bounds mapBounds,
	assets videoAssets,
) {
	if match == nil {
		return
	}

	for entityID, entity := range match.Entities {
		if len(entity.Positions) == 0 || entity.Objective == nil {
			continue
		}

		var (
			status      renderEntityStatus = statuses[entityID]
			markerColor color.RGBA         = teamColor(match, status.teamID, 190)
			point       image.Point        = mapPoint(entity.Positions[0].Position, bounds, frame.Bounds())
			radius      int                = int(status.objectiveRadius *
				float64(frame.Bounds().Dx()) /
				float64(bounds.maxX-bounds.minX))
		)

		if !status.objectiveEnabled || !status.objectiveVisible {
			continue
		}

		radius = max(radius, 12)
		drawRing(frame, point, radius, markerColor)

		if status.hasInvaders && status.captureProgress > 0 {
			drawProgressArc(
				frame,
				point,
				max(radius-3, 4),
				status.captureProgress,
				teamColor(match, status.invaderTeam, 255),
			)
		}

		drawCenteredText(
			frame,
			assets,
			true,
			point.X,
			point.Y+4,
			string(rune('A'+status.objectiveIndex)),
			color.RGBA{R: 245, G: 248, B: 250, A: 255},
		)
	}
}

// teamColor maps a canonical team ID into the local ally/enemy palette.
func teamColor(match *Match, teamID int32, alpha uint8) (markerColor color.RGBA) {
	var (
		found   bool
		ownShip *MatchEntity
	)

	markerColor = color.RGBA{R: 220, G: 225, B: 228, A: alpha}

	if ownShip, found = match.Entities[match.OwnShipID]; found && ownShip.TeamKnown && teamID >= 0 {
		if teamID == ownShip.TeamID {
			markerColor = color.RGBA{R: 76, G: 214, B: 116, A: alpha}
		} else {
			markerColor = color.RGBA{R: 255, G: 92, B: 92, A: alpha}
		}
	}

	return
}

// drawProgressArc renders capture progress clockwise from twelve o'clock.
func drawProgressArc(
	frame *image.RGBA,
	center image.Point,
	radius int,
	progress float64,
	arcColor color.RGBA,
) {
	var steps int = max(int(min(max(progress, 0), 1)*96), 1)

	for step := 0; step <= steps; step++ {
		var angle float64 = -math.Pi/2 + float64(step)*2*math.Pi/96

		for thickness := -1; thickness <= 1; thickness++ {
			var point image.Point = image.Pt(
				center.X+int(math.Cos(angle)*float64(radius+thickness)),
				center.Y+int(math.Sin(angle)*float64(radius+thickness)),
			)

			if point.In(frame.Bounds()) {
				frame.SetRGBA(point.X, point.Y, arcColor)
			}
		}
	}
}

// drawRing rasterizes a continuous objective-zone outline.
func drawRing(frame *image.RGBA, center image.Point, radius int, ringColor color.RGBA) {
	var steps int = max(int(2*math.Pi*float64(radius)*1.5), 48)

	for step := 0; step < steps; step++ {
		var angle float64 = float64(step) * 2 * math.Pi / float64(steps)

		for thickness := 0; thickness <= 1; thickness++ {
			var point image.Point = image.Pt(
				center.X+int(math.Cos(angle)*float64(radius-thickness)),
				center.Y+int(math.Sin(angle)*float64(radius-thickness)),
			)

			if point.In(frame.Bounds()) {
				frame.SetRGBA(point.X, point.Y, ringColor)
			}
		}
	}
}

// drawVideoText renders one antialiased label at a baseline coordinate.
func drawVideoText(
	frame *image.RGBA,
	assets videoAssets,
	bold bool,
	left,
	baseline int,
	text string,
	textColor color.RGBA,
) {
	var cached cachedVideoText = assets.cachedText(bold, text, textColor)

	if cached.image == nil {
		return
	}

	draw.Draw(
		frame,
		image.Rect(
			left,
			baseline-cached.ascent,
			left+cached.image.Bounds().Dx(),
			baseline-cached.ascent+cached.image.Bounds().Dy(),
		),
		cached.image,
		image.Point{},
		draw.Over,
	)
}

// drawCenteredText horizontally centers one antialiased label on a point.
func drawCenteredText(
	frame *image.RGBA,
	assets videoAssets,
	bold bool,
	center,
	baseline int,
	text string,
	textColor color.RGBA,
) {
	var cached cachedVideoText = assets.cachedText(bold, text, textColor)

	if cached.image == nil {
		return
	}

	drawVideoText(frame, assets, bold, center-cached.image.Bounds().Dx()/2, baseline, text, textColor)
}

// fitVideoText truncates a proportional-font label to an exact pixel width.
func fitVideoText(assets videoAssets, bold bool, value string, maximumWidth int) (label string) {
	var (
		cacheKey textFitKey = textFitKey{text: value, width: maximumWidth, bold: bold}
		face     font.Face  = assets.regularFace
		found    bool
	)

	if label, found = assets.fitCache[cacheKey]; found {
		return
	}
	if bold {
		face = assets.boldFace
	}
	label = value
	if face == nil {
		return
	}

	var drawer font.Drawer = font.Drawer{Face: face}

	if drawer.MeasureString(label).Round() <= maximumWidth {
		assets.fitCache[cacheKey] = label

		return
	}

	var characters []rune = []rune(label)

	for len(characters) > 0 {
		characters = characters[:len(characters)-1]
		label = string(characters) + "..."
		if drawer.MeasureString(label).Round() <= maximumWidth {
			assets.fitCache[cacheKey] = label

			return
		}
	}
	label = ""
	assets.fitCache[cacheKey] = label

	return
}

// cachedText rasterizes each static label once for the entire video.
func (assets videoAssets) cachedText(
	bold bool,
	text string,
	textColor color.RGBA,
) (cached cachedVideoText) {
	var (
		cacheKey textCacheKey = textCacheKey{text: text, color: textColor, bold: bold}
		face     font.Face    = assets.regularFace
		found    bool
	)

	if cached, found = assets.textCache[cacheKey]; found {
		return
	}
	if bold {
		face = assets.boldFace
	}
	if face == nil || text == "" {
		return
	}

	var (
		drawer font.Drawer = font.Drawer{Face: face}
		height int         = max(face.Metrics().Height.Ceil(), 1)
		width  int         = max(drawer.MeasureString(text).Ceil(), 1)
	)

	cached = cachedVideoText{
		image:  image.NewRGBA(image.Rect(0, 0, width, height)),
		ascent: face.Metrics().Ascent.Ceil(),
	}
	drawer.Dst = cached.image
	drawer.Src = image.NewUniform(textColor)
	drawer.Dot = fixed.P(0, cached.ascent)
	drawer.DrawString(text)
	assets.textCache[cacheKey] = cached

	return
}

// drawTeamLineups renders complete metadata rosters with live health on both sides.
func drawTeamLineups(
	frame *image.RGBA,
	match *Match,
	statuses map[uint32]renderEntityStatus,
	assets videoAssets,
) {
	if match == nil || len(match.Participants) == 0 {
		return
	}

	var (
		allies  []MatchParticipant
		enemies []MatchParticipant
	)

	for _, participant := range match.Participants {
		if participant.Relation == 2 {
			enemies = append(enemies, participant)
		} else {
			allies = append(allies, participant)
		}
	}

	sortLineupParticipants(allies)
	sortLineupParticipants(enemies)

	drawTeamLineup(frame, match, statuses, assets, allies, false)
	drawTeamLineup(frame, match, statuses, assets, enemies, true)
}

// sortLineupParticipants applies WoWS class, nation, ship, and player ordering.
func sortLineupParticipants(participants []MatchParticipant) {
	sort.SliceStable(participants, func(left, right int) bool {
		var (
			leftClass   int = lineupClassOrder(participants[left].ShipClass)
			rightClass  int = lineupClassOrder(participants[right].ShipClass)
			leftNation  int = lineupNationOrder(participants[left].Nation)
			rightNation int = lineupNationOrder(participants[right].Nation)
		)

		if leftClass != rightClass {
			return leftClass < rightClass
		}
		if leftNation != rightNation {
			return leftNation < rightNation
		}
		if strings.ToLower(participants[left].ShipName) != strings.ToLower(participants[right].ShipName) {
			return strings.ToLower(participants[left].ShipName) <
				strings.ToLower(participants[right].ShipName)
		}

		return strings.ToLower(participants[left].PlayerName) <
			strings.ToLower(participants[right].PlayerName)
	})
}

// lineupClassOrder returns carrier, battleship, cruiser, destroyer, submarine order.
func lineupClassOrder(class string) (order int) {
	switch normalizeLineupValue(class) {
	case "aircarrier", "carrier", "cv":
		order = 0
	case "battleship", "bb":
		order = 1
	case "cruiser", "ca", "cl":
		order = 2
	case "destroyer", "dd":
		order = 3
	case "submarine", "ss":
		order = 4
	default:
		order = 5
	}

	return
}

// lineupNationOrder applies the requested nation priority within each class.
func lineupNationOrder(nation string) (order int) {
	switch normalizeLineupValue(nation) {
	case "japan":
		order = 0
	case "usa", "unitedstates":
		order = 1
	case "russia", "ussr", "soviet", "sovietunion":
		order = 2
	case "germany":
		order = 3
	case "uk", "unitedkingdom", "britain":
		order = 4
	case "france":
		order = 5
	case "italy":
		order = 6
	case "panasia":
		order = 7
	case "europe", "european":
		order = 8
	case "netherlands", "dutch":
		order = 9
	case "commonwealth":
		order = 10
	case "panamerica", "panamerican":
		order = 11
	case "spain":
		order = 12
	default:
		order = 13
	}

	return
}

// normalizeLineupValue removes separators and case differences from catalog values.
func normalizeLineupValue(value string) (normalized string) {
	normalized = strings.ToLower(value)
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")

	return
}

// drawTeamLineup renders one roster column.
func drawTeamLineup(
	frame *image.RGBA,
	match *Match,
	statuses map[uint32]renderEntityStatus,
	assets videoAssets,
	participants []MatchParticipant,
	right bool,
) {
	var (
		panelWidth int = assets.pixels(184)
		rowHeight  int = assets.pixels(31)
		left       int = assets.pixels(6)
		maxRows    int = (frame.Bounds().Dy() - assets.pixels(42)) / rowHeight
		titleColor color.RGBA
	)

	if right {
		left = frame.Bounds().Dx() - panelWidth - assets.pixels(6)
		titleColor = color.RGBA{R: 255, G: 118, B: 118, A: 255}
		drawVideoText(
			frame,
			assets,
			true,
			left+assets.pixels(4),
			assets.pixels(16),
			"ENEMIES",
			titleColor,
		)
	} else {
		titleColor = color.RGBA{R: 98, G: 226, B: 132, A: 255}
		drawVideoText(
			frame,
			assets,
			true,
			left+assets.pixels(4),
			assets.pixels(16),
			"ALLIES",
			titleColor,
		)
	}

	for index := 0; index < len(participants) && index < maxRows; index++ {
		var (
			participant MatchParticipant   = participants[index]
			status      renderEntityStatus = renderEntityStatus{alive: true}
			top         int                = assets.pixels(22) + index*rowHeight
		)

		if participant.EntityID != 0 {
			status = statuses[participant.EntityID]
		}

		draw.Draw(
			frame,
			image.Rect(left, top, left+panelWidth, top+rowHeight-assets.pixels(2)),
			&image.Uniform{C: color.RGBA{R: 3, G: 10, B: 14, A: 170}},
			image.Point{},
			draw.Over,
		)
		drawRosterParticipant(frame, assets, participant, status, left, top, panelWidth, right)
	}
}

// drawRosterParticipant draws one class silhouette, two labels, and a health bar.
func drawRosterParticipant(
	frame *image.RGBA,
	assets videoAssets,
	participant MatchParticipant,
	status renderEntityStatus,
	left,
	top,
	width int,
	right bool,
) {
	var (
		icon      *image.RGBA
		iconLeft  int    = left + assets.pixels(2)
		relation  uint32 = participant.Relation
		species   string = strings.ToLower(participant.ShipClass)
		textColor color.RGBA
		textLeft  int = left + assets.pixels(29)
	)

	if species == "" {
		species = "cruiser"
	}
	if icon = assets.icons[shipIconKey(relation, species, !status.alive)]; icon == nil {
		icon = assets.icons[shipIconKey(relation, "cruiser", !status.alive)]
	}
	if right {
		iconLeft = left + width - assets.pixels(26)
		textLeft = left + assets.pixels(4)
	}

	if relation == 2 {
		textColor = color.RGBA{R: 255, G: 142, B: 142, A: 255}
	} else {
		textColor = color.RGBA{R: 132, G: 232, B: 154, A: 255}
	}
	if !status.alive {
		textColor = color.RGBA{R: 125, G: 125, B: 125, A: 255}
	}

	if icon != nil {
		draw.Draw(
			frame,
			image.Rect(iconLeft, top, iconLeft+icon.Bounds().Dx(), top+icon.Bounds().Dy()),
			icon,
			image.Point{},
			draw.Over,
		)
	}

	var maximumTextWidth int = width - assets.pixels(35)

	drawVideoText(
		frame,
		assets,
		true,
		textLeft,
		top+assets.pixels(11),
		fitVideoText(assets, true, participant.PlayerName, maximumTextWidth),
		textColor,
	)
	drawVideoText(
		frame,
		assets,
		false,
		textLeft,
		top+assets.pixels(22),
		fitVideoText(assets, false, participant.ShipName, maximumTextWidth),
		color.RGBA{R: 190, G: 198, B: 202, A: 255},
	)
	drawRosterHealth(
		frame,
		textLeft,
		top+assets.pixels(26),
		maximumTextWidth,
		assets.pixels(2),
		status,
		textColor,
	)
}

// drawRosterHealth renders one participant's known current health.
func drawRosterHealth(
	frame *image.RGBA,
	left,
	top,
	width,
	height int,
	status renderEntityStatus,
	barColor color.RGBA,
) {
	draw.Draw(
		frame,
		image.Rect(left, top, left+width, top+height),
		&image.Uniform{C: color.RGBA{R: 36, G: 43, B: 47, A: 230}},
		image.Point{},
		draw.Over,
	)

	if status.maxHealth <= 0 {
		return
	}

	var filled int = int(float64(width) * max(min(status.health/status.maxHealth, 1), 0))

	if !status.alive {
		filled = 0
	}
	draw.Draw(
		frame,
		image.Rect(left, top, left+filled, top+height),
		&image.Uniform{C: barColor},
		image.Point{},
		draw.Over,
	)
}

// matchEntityColor uses replay relations when available and stable fallback colors otherwise.
func matchEntityColor(match *Match, entityID uint32, ownShip bool) (entityColor color.RGBA) {
	if ownShip {
		entityColor = color.RGBA{R: 255, G: 214, B: 64, A: 255}

		return
	}

	if match != nil {
		if relation, found := relationForEntity(match, entityID); found {
			switch relation {
			case 1:
				entityColor = color.RGBA{R: 76, G: 214, B: 116, A: 255}

				return
			case 2:
				entityColor = color.RGBA{R: 255, G: 88, B: 88, A: 255}

				return
			}
		}
	}

	entityColor = colorForEntity(entityID, false)

	return
}

// iconForEntity returns the class-, relation-, and life-state-appropriate ship icon.
func iconForEntity(
	assets videoAssets,
	match *Match,
	entityID uint32,
	sunk bool,
) (icon *image.RGBA) {
	if match == nil {
		return
	}

	var (
		entity        *MatchEntity
		found         bool
		relation      uint32
		relationFound bool
	)

	if entity, found = match.Entities[entityID]; !found || entity.Type != "Vehicle" {
		return
	}

	var species string = strings.ToLower(entity.ShipClass)

	if species == "" {
		species = shipSpeciesFromIndex(match, entity)
	}
	if species == "" {
		species = "cruiser"
	}

	relation = entity.Relation
	if relation, relationFound = relationForEntity(match, entityID); relationFound {
		icon = assets.icons[shipIconKey(relation, species, sunk)]
	}
	if icon == nil {
		icon = assets.icons[shipIconKey(entity.Relation, species, sunk)]
	}
	if icon == nil {
		icon = assets.icons[shipIconKey(relation, "cruiser", sunk)]
	}

	return
}

// shipIconKey builds the internal asset lookup key.
func shipIconKey(relation uint32, species string, sunk bool) (key string) {
	key = fmt.Sprintf("%d:%s:%t", relation, species, sunk)

	return
}

// shipIconYaw normalizes enemy artwork, which natively faces left instead of right.
func shipIconYaw(match *Match, entityID uint32, yaw float32) (iconYaw float32) {
	var (
		entity   *MatchEntity
		found    bool
		relation uint32
	)

	iconYaw = yaw - math.Pi/2
	if relation, found = relationForEntity(match, entityID); !found {
		if entity, found = match.Entities[entityID]; found {
			relation = entity.Relation
		}
	}

	if relation == 2 {
		iconYaw += math.Pi
	}

	return
}

// shipSpeciesFromIndex recovers the recording ship class from its metadata index.
func shipSpeciesFromIndex(match *Match, entity *MatchEntity) (species string) {
	if entity.ID != match.OwnShipID {
		return
	}

	var index string = filepath.Base(match.Metadata.PlayerVehicle)

	if len(index) < 4 {
		return
	}

	switch index[3] {
	case 'A':
		species = "aircarrier"
	case 'B':
		species = "battleship"
	case 'C':
		species = "cruiser"
	case 'D':
		species = "destroyer"
	case 'S':
		species = "submarine"
	}

	return
}

// drawRotatedIcon composites a small icon around its center with yaw and opacity.
func drawRotatedIcon(
	frame *image.RGBA,
	icon *image.RGBA,
	center image.Point,
	yaw float32,
	opacity uint8,
) {
	var (
		cosine float64 = math.Cos(float64(yaw))
		halfX  int     = icon.Bounds().Dx() / 2
		halfY  int     = icon.Bounds().Dy() / 2
		sine   float64 = math.Sin(float64(yaw))
	)

	for targetY := -halfY; targetY <= halfY; targetY++ {
		for targetX := -halfX; targetX <= halfX; targetX++ {
			var (
				sourceX int         = int(math.Round(float64(targetX)*cosine+float64(targetY)*sine)) + halfX
				sourceY int         = int(math.Round(-float64(targetX)*sine+float64(targetY)*cosine)) + halfY
				target  image.Point = image.Pt(center.X+targetX, center.Y+targetY)
			)

			if sourceX < 0 || sourceX >= icon.Bounds().Dx() ||
				sourceY < 0 || sourceY >= icon.Bounds().Dy() ||
				!target.In(frame.Bounds()) {
				continue
			}

			var source color.RGBA = icon.RGBAAt(sourceX, sourceY)

			source.A = uint8(uint16(source.A) * uint16(opacity) / 255)
			blendRGBA(frame, target, source)
		}
	}
}

// blendRGBA composites one straight-alpha pixel over a frame.
func blendRGBA(frame *image.RGBA, point image.Point, source color.RGBA) {
	if source.A == 0 {
		return
	}

	var (
		destination color.RGBA = frame.RGBAAt(point.X, point.Y)
		alpha       uint32     = uint32(source.A)
		inverse     uint32     = 255 - alpha
	)

	frame.SetRGBA(point.X, point.Y, color.RGBA{
		R: uint8((uint32(source.R)*alpha + uint32(destination.R)*inverse) / 255),
		G: uint8((uint32(source.G)*alpha + uint32(destination.G)*inverse) / 255),
		B: uint8((uint32(source.B)*alpha + uint32(destination.B)*inverse) / 255),
		A: 255,
	})
}

// relationForEntity derives own/ally/enemy from canonical team IDs when possible.
func relationForEntity(match *Match, entityID uint32) (relation uint32, found bool) {
	if entityID == match.OwnShipID {
		found = true

		return
	}

	var (
		entity  *MatchEntity
		ownShip *MatchEntity
	)

	if entity, found = match.Entities[entityID]; !found || !entity.TeamKnown {
		found = false

		return
	}

	if ownShip, found = match.Entities[match.OwnShipID]; !found || !ownShip.TeamKnown {
		found = false

		return
	}

	if entity.TeamID == ownShip.TeamID {
		relation = 1
	} else {
		relation = 2
	}

	return
}

// drawHeadingMarker draws a small oriented triangle when no imported icon is available.
func drawHeadingMarker(
	frame *image.RGBA,
	center image.Point,
	radius int,
	yaw float32,
	markerColor color.RGBA,
) {
	var (
		directionX float64     = math.Sin(float64(yaw))
		directionY float64     = -math.Cos(float64(yaw))
		tip        image.Point = image.Pt(
			center.X+int(directionX*float64(radius+3)),
			center.Y+int(directionY*float64(radius+3)),
		)
	)

	drawCircle(frame, center, radius, markerColor)
	drawLine(frame, center, tip, markerColor)
}

// drawHealthBar renders known current health directly beneath a ship marker.
func drawHealthBar(
	frame *image.RGBA,
	center image.Point,
	status renderEntityStatus,
	barColor color.RGBA,
) {
	if status.maxHealth <= 0 {
		return
	}

	var (
		width  int = 22
		filled int = int(float64(width) * max(min(status.health/status.maxHealth, 1), 0))
		top    int = center.Y + 13
		left   int = center.X - width/2
	)

	draw.Draw(
		frame,
		image.Rect(left, top, left+width, top+3),
		&image.Uniform{C: color.RGBA{R: 20, G: 24, B: 28, A: 220}},
		image.Point{},
		draw.Over,
	)

	if !status.alive {
		barColor = color.RGBA{R: 90, G: 90, B: 90, A: 220}
	}

	draw.Draw(
		frame,
		image.Rect(left, top, left+filled, top+3),
		&image.Uniform{C: barColor},
		image.Point{},
		draw.Over,
	)
}

// drawGrid adds a subtle reference grid.
func drawGrid(frame *image.RGBA) {
	var gridColor color.RGBA = color.RGBA{R: 25, G: 48, B: 60, A: 255}

	for division := 1; division < 8; division++ {
		var (
			x int = division * frame.Bounds().Dx() / 8
			y int = division * frame.Bounds().Dy() / 8
		)

		drawLine(frame, image.Pt(x, 0), image.Pt(x, frame.Bounds().Dy()-1), gridColor)
		drawLine(frame, image.Pt(0, y), image.Pt(frame.Bounds().Dx()-1, y), gridColor)
	}
}

// mapPoint converts BigWorld X/Z coordinates to image coordinates.
func mapPoint(position Vector3, bounds mapBounds, rectangle image.Rectangle) (point image.Point) {
	var (
		x float32 = (position.X - bounds.minX) / (bounds.maxX - bounds.minX)
		z float32 = (position.Z - bounds.minZ) / (bounds.maxZ - bounds.minZ)
	)

	point = image.Pt(
		int(x*float32(rectangle.Dx()-1)),
		int((1-z)*float32(rectangle.Dy()-1)),
	)

	return
}

// colorForEntity returns a stable, high-contrast entity color.
func colorForEntity(entityID uint32, ownShip bool) (entityColor color.RGBA) {
	if ownShip {
		entityColor = color.RGBA{R: 255, G: 214, B: 64, A: 255}

		return
	}

	var palette []color.RGBA = []color.RGBA{
		{R: 67, G: 181, B: 255, A: 255},
		{R: 255, G: 99, B: 110, A: 255},
		{R: 85, G: 214, B: 137, A: 255},
		{R: 208, G: 135, B: 255, A: 255},
		{R: 255, G: 159, B: 67, A: 255},
	}
	entityColor = palette[int(entityID)%len(palette)]

	return
}

// drawLine rasterizes a line with integer Bresenham stepping.
func drawLine(frame *image.RGBA, from, to image.Point, lineColor color.RGBA) {
	var (
		deltaX     int = abs(to.X - from.X)
		deltaY     int = -abs(to.Y - from.Y)
		errorValue int = deltaX + deltaY
		stepX      int = -1
		stepY      int = -1
	)

	if from.X < to.X {
		stepX = 1
	}

	if from.Y < to.Y {
		stepY = 1
	}

	for {
		if from.In(frame.Bounds()) {
			blendRGBA(frame, from, lineColor)
		}

		if from == to {
			break
		}

		var doubledError int = 2 * errorValue

		if doubledError >= deltaY {
			errorValue += deltaY
			from.X += stepX
		}

		if doubledError <= deltaX {
			errorValue += deltaX
			from.Y += stepY
		}
	}
}

// drawCircle rasterizes a filled entity marker.
func drawCircle(frame *image.RGBA, center image.Point, radius int, circleColor color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			var point image.Point = image.Pt(center.X+x, center.Y+y)

			if x*x+y*y <= radius*radius && point.In(frame.Bounds()) {
				blendRGBA(frame, point, circleColor)
			}
		}
	}
}

// validMapPosition rejects corrupt, non-finite, and sentinel coordinates.
func validMapPosition(position Vector3) (valid bool) {
	valid = !math.IsNaN(float64(position.X)) &&
		!math.IsNaN(float64(position.Z)) &&
		!math.IsInf(float64(position.X), 0) &&
		!math.IsInf(float64(position.Z), 0) &&
		math.Abs(float64(position.X)) < 1e7 &&
		math.Abs(float64(position.Z)) < 1e7

	return
}

// abs returns the magnitude of an integer.
func abs(value int) (absolute int) {
	absolute = value
	if absolute < 0 {
		absolute = -absolute
	}

	return
}
