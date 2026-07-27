package wowsreplay

import (
	"image"
	"image/color"
	"math"
)

// clipTorpedoTraces ends projected tracks at the first visible terrain intersection.
func clipTorpedoTraces(traces []WeaponTrace, bounds mapBounds, terrain *image.RGBA) {
	if terrain == nil {
		return
	}

	for index := range traces {
		if traces[index].Kind != "torpedo" {
			continue
		}

		var (
			end       image.Point = mapPoint(traces[index].End, bounds, terrain.Bounds())
			start     image.Point = mapPoint(traces[index].Start, bounds, terrain.Bounds())
			steps     int         = max(abs(end.X-start.X), abs(end.Y-start.Y))
			seenWater bool
		)

		for step := 0; step <= steps; step++ {
			var (
				ratio float32     = float32(step) / float32(max(steps, 1))
				point image.Point = mapPoint(
					interpolateTracePosition(traces[index], ratio),
					bounds,
					terrain.Bounds(),
				)
			)

			if !point.In(terrain.Bounds()) {
				continue
			}
			if terrain.RGBAAt(point.X, point.Y).A <= 32 {
				seenWater = true

				continue
			}
			if !seenWater || step <= 2 {
				continue
			}

			ratio = max(float32(step-1)/float32(max(steps, 1)), 0)
			traces[index].End = interpolateTracePosition(traces[index], ratio)
			traces[index].Duration *= ratio
			traces[index].Range *= ratio

			break
		}
	}
}

// retainActiveTraces drops ammunition after its transmitted flight duration.
func retainActiveTraces(traces []WeaponTrace, clock float32) (active []WeaponTrace) {
	active = traces[:0]

	for _, trace := range traces {
		if clock <= trace.Clock+traceDisplayDuration(trace) {
			active = append(active, trace)
		}
	}

	return
}

// traceDisplayDuration returns the transmitted ammunition lifetime without lingering.
func traceDisplayDuration(trace WeaponTrace) (duration float32) {
	duration = max(trace.Duration, 0.1)
	if trace.Kind == "explosion" {
		duration = min(duration, 0.8)
	}

	return
}

// drawWeaponTrace draws ammunition at its transmitted position and lifetime.
func drawWeaponTrace(
	frame *image.RGBA,
	trace WeaponTrace,
	bounds mapBounds,
	clock float32,
	match *Match,
) {
	var (
		elapsed float32 = max(clock-trace.Clock, 0)
		ratio   float32 = min(elapsed/max(trace.Duration, 0.1), 1)
		current Vector3 = interpolateTracePosition(trace, ratio)
	)

	switch trace.Kind {
	case "torpedo":
		drawTorpedoTrace(frame, trace, bounds, current, match)
	case "shell":
		drawShellTrace(frame, trace, bounds, current, ratio)
	case "explosion":
		var fade float32 = max(1-elapsed/traceDisplayDuration(trace), 0)

		drawCircle(
			frame,
			mapPoint(current, bounds, frame.Bounds()),
			2+int(5*ratio),
			color.RGBA{R: 255, G: 80, B: 52, A: uint8(240 * fade)},
		)
	default:
		drawGenericTrace(frame, trace, bounds, current)
	}
}

// drawGenericTrace renders non-shell ordnance using its transmitted path.
func drawGenericTrace(
	frame *image.RGBA,
	trace WeaponTrace,
	bounds mapBounds,
	current Vector3,
) {
	var (
		point      image.Point = mapPoint(current, bounds, frame.Bounds())
		traceColor color.RGBA
	)

	switch trace.Kind {
	case "aircraft":
		traceColor = color.RGBA{R: 255, G: 187, B: 71, A: 230}
	case "depth_charge":
		traceColor = color.RGBA{R: 173, G: 112, B: 255, A: 230}
	default:
		traceColor = color.RGBA{R: 255, G: 238, B: 174, A: 220}
	}

	drawLine(frame, mapPoint(trace.Start, bounds, frame.Bounds()), point, traceColor)
	drawCircle(frame, point, 2, traceColor)
}

// drawShellTrace renders a tiny projectile with a short rapidly fading tail.
func drawShellTrace(
	frame *image.RGBA,
	trace WeaponTrace,
	bounds mapBounds,
	current Vector3,
	ratio float32,
) {
	var (
		point          image.Point = mapPoint(current, bounds, frame.Bounds())
		projectileSize int         = 1
		tailSeconds    float32     = 0.3
	)

	if trace.Secondary {
		projectileSize = 0
		tailSeconds = 0.15
	}

	const trailSegments int = 4

	for segment := 0; segment < trailSegments; segment++ {
		var (
			fromAge   float32     = tailSeconds * float32(segment+1) / float32(trailSegments)
			toAge     float32     = tailSeconds * float32(segment) / float32(trailSegments)
			fromRatio float32     = max(ratio-fromAge/max(trace.Duration, 0.1), 0)
			toRatio   float32     = max(ratio-toAge/max(trace.Duration, 0.1), 0)
			from      image.Point = mapPoint(
				interpolateTracePosition(trace, fromRatio),
				bounds,
				frame.Bounds(),
			)
			to image.Point = mapPoint(
				interpolateTracePosition(trace, toRatio),
				bounds,
				frame.Bounds(),
			)
			alpha uint8 = uint8(28 + segment*28)
		)

		drawLine(frame, from, to, color.RGBA{R: 255, G: 238, B: 174, A: alpha})
	}
	drawCircle(frame, point, projectileSize, color.RGBA{R: 255, G: 248, B: 220, A: 235})
}

// drawTorpedoTrace renders an unmistakable inverted triangle and tiny directional wake.
func drawTorpedoTrace(
	frame *image.RGBA,
	trace WeaponTrace,
	bounds mapBounds,
	current Vector3,
	match *Match,
) {
	var (
		deltaX       float64     = float64(trace.End.X - trace.Start.X)
		deltaY       float64     = -float64(trace.End.Z - trace.Start.Z)
		length       float64     = math.Hypot(deltaX, deltaY)
		point        image.Point = mapPoint(current, bounds, frame.Bounds())
		torpedoColor color.RGBA  = torpedoTraceColor(match, trace)
	)

	if length <= 0 {
		deltaY = 1
		length = 1
	}

	var (
		directionX float64     = deltaX / length
		directionY float64     = deltaY / length
		trailEnd   image.Point = image.Pt(
			point.X-int(math.Round(directionX*5)),
			point.Y-int(math.Round(directionY*5)),
		)
		trailColor color.RGBA  = torpedoColor
		tip        image.Point = image.Pt(point.X, point.Y+3)
		left       image.Point = image.Pt(point.X-3, point.Y-2)
		right      image.Point = image.Pt(point.X+3, point.Y-2)
	)

	trailColor.A = 75
	drawLine(frame, trailEnd, point, trailColor)
	drawLine(frame, tip, left, torpedoColor)
	drawLine(frame, left, right, torpedoColor)
	drawLine(frame, right, tip, torpedoColor)
	drawLine(
		frame,
		image.Pt(point.X-2, point.Y-1),
		image.Pt(point.X+2, point.Y-1),
		torpedoColor,
	)
}

// torpedoTraceColor maps the launcher relation to WoWS torpedo colors.
func torpedoTraceColor(match *Match, trace WeaponTrace) (traceColor color.RGBA) {
	traceColor = color.RGBA{R: 242, G: 242, B: 242, A: 245}
	if match == nil {
		return
	}
	if trace.OwnerID == int64(match.OwnShipID) {
		traceColor = color.RGBA{R: 75, G: 228, B: 111, A: 245}

		return
	}

	if relation, found := relationForEntity(match, uint32(trace.OwnerID)); found && relation == 2 {
		traceColor = color.RGBA{R: 255, G: 82, B: 82, A: 245}
	}

	return
}

// interpolateTracePosition returns one point along a transmitted ammunition path.
func interpolateTracePosition(trace WeaponTrace, ratio float32) (position Vector3) {
	position = Vector3{
		X: trace.Start.X + (trace.End.X-trace.Start.X)*ratio,
		Y: trace.Start.Y + (trace.End.Y-trace.Start.Y)*ratio,
		Z: trace.Start.Z + (trace.End.Z-trace.Start.Z)*ratio,
	}

	return
}
