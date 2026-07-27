# WoWS Replay Parser

`wowsreplay` is an isolated Go package for reading World of Warships
`.wowsreplay` files. It is not wired into Project Hawaii and keeps its external
dependencies limited to replay pickle decoding, Blowfish, and font rendering.

The container parser accepts an `io.Reader`, byte slice, or path. It validates
safety limits, reads every metadata block, decrypts the Blowfish packet stream,
decompresses it, and walks every packet without requiring an installed game
client.

## Version compatibility

The parser is not capped at the newest fixture in its test suite. Versions
12.6 and newer use the modern packet-ID layout; unknown packet identifiers are
preserved instead of causing a parse failure. The local real-replay suite
currently validates WoWS 15.6.

```go
replay, err := wowsreplay.Parse(uploadReader)
if err != nil {
    return err
}

if err = replay.WriteEventLog(output, wowsreplay.LogOptions{
    Format: wowsreplay.LogFormatJSONLines
}); err != nil {
    return err
}
```

Path and byte-slice entry points are also available:

```go
replay, err := wowsreplay.ParseFile("battle.wowsreplay")
replay, err := wowsreplay.ParseBytes(upload)
```

## Outputs

`WriteEventLog` supports:

- JSON Lines, with a metadata record followed by one record per packet.
- Compact text intended for debugging and inspection.
- Optional raw packet payloads. JSON encodes these bytes as base64; text uses
  hexadecimal.

All packet headers and payloads are preserved. Definition-independent packet
types have typed `Event.Data`, including entity positions/creation, timestamps,
camera state, client network stats, own-ship selection, weapon locks, and battle
results. Unknown packets remain in the event stream with names such as
`Unknown_0x34`. `DecompressedPackets` returns a copy of the complete low-level
packet stream for tooling that needs to walk it independently.

`RenderVideo` writes a path and `WriteVideo` streams to any `io.Writer`. Without
resources, video output remains a deliberately simple tactical plot. Supplying
an imported resource bundle enables the complete renderer layer:

- The real island minimap, drawn over the reference grid, with WoWS's
  map-border parameter applied as the mode-specific playable crop.
- Rotating, class-correct own, allied, and enemy ship icons loaded from the
  matching client resources, with the ship name rendered beneath each icon.
- Health bars, persistent wrecks, last-known fog-of-war markers, and movement
  trails. Full-precision player orientation and packed minimap contacts are
  both reconstructed.
- Correctly scaled and labeled capture zones with historical ownership,
  invader, and capture-progress state.
- Complete allied/enemy side lineups with class silhouettes, player and ship
  names, destruction state, and live known health. Lineups sort by carrier,
  battleship, cruiser, destroyer, and submarine, then by the configured nation
  priority.
- Full-flight main and smaller secondary shells with short fading tracer tails
  and their transmitted impact endpoints.
- Directional torpedo triangles colored green for the player, white for
  teammates, and red for enemies. Their speed and range come from GameParams,
  visibility starts at the received spotting event, and terrain intersections
  end the track.

Defaults are 960x960, 24 FPS with interpolated movement, 4x playback, H.264 CRF 30,
and no audio. Imported resources provide the Warhelios UI font, with a clean
embedded fallback when they are unavailable. This keeps
files small while retaining the tactical state.

`PlaybackSpeed` is an exact replay-time multiplier: `1` is normal game speed,
`4` is four-times speed, and `10` is ten-times speed. `Width` and `Height`
control output resolution, and tactical overlays scale with them.

```go
options := wowsreplay.DefaultVideoOptions()
options.Width = 1440
options.Height = 1440
options.PlaybackSpeed = 10
options.Resources = resources

if err = replay.RenderVideo(ctx, "battle.mp4", options); err != nil {
    return err
}
```

Video output requires `ffmpeg` with `libx264` on `PATH`. Set
`VideoOptions.FFmpegPath` when it is installed elsewhere.

## Batch export

The included command exports every replay in a directory to a raw-payload JSONL
event log and compact MP4, then writes `manifest.json`:

```powershell
go run ./wowsreplay/cmd/wowsreplay-export `
    -input wowsreplay/testdata/replays `
    -output wowsreplay/testdata/exports
```

Use `-video=false` for logs only or `-raw-payload=false` for substantially
smaller semantic-only logs.

Pass the exact imported bundle to produce semantic logs, reconstructed match
JSON, and resource-backed video:

```powershell
go run ./wowsreplay/cmd/wowsreplay-export `
    -input wowsreplay/testdata/replays `
    -output wowsreplay/testdata/exports `
    -resources wowsreplay/testdata/game/15.6.0_12830008
```

`-input` may also name one replay file.

## Version-specific semantic data

WoWS entity method and property payloads depend on RPC definitions shipped with
the exact client build. The package currently preserves these payloads
losslessly but does not assign method/property names or decode their arguments.
Position-based video rendering does not need those definitions.

## Importing game resources

`wowsreplay-import` is a native Go importer for an installed WoWS client. It
reads the current client's IDX/PKG archives directly, so it does not require
Rust, Administrator mode, symlink privileges, or a separately downloaded
unpacking utility.

From the `server` directory, the installation directory is the only required
argument:

```powershell
go run ./wowsreplay/cmd/wowsreplay-import `
    "C:\Program Files (x86)\Steam\steamapps\common\World of Warships"
```

The equivalent explicit form is:

```powershell
go run ./wowsreplay/cmd/wowsreplay-import `
    -install "C:\Games\World_of_Warships" `
    -output wowsreplay/testdata/game
```

The newest numeric build under `bin` is selected automatically. Use `-build`
for a specific installed build and `-force` to replace its existing bundle.
Each output is stored as `<version>_<build>/` and includes:

- Raw game parameters and RPC entity definitions for semantic event decoding.
- Map minimaps, water layers, and settings for true terrain and coordinates.
- Ship, aircraft, HUD, consumable, ribbon, flag, and other tactical icons.
- Available locale translation catalogs.

`OpenResourceBundle` and `FindResourceBundle` validate and expose the imported
tree. The data is copied into ordinary files, making the bundle portable and
safe to use after the game installation changes.

`NewSemanticDecoder` loads the imported XML aliases, interfaces, entity
properties, and exposed client methods. Its recursive wire decoder supports
primitives, named types, arrays, tuples, nullable fixed dictionaries, and
converter-backed user types. Method IDs are derived using BigWorld's exposed
method ordering rules rather than hard-coded to WoWS 15.6.

Use `ParseFileWithResources` for the convenient version-checked entry point:

```go
resources, err := wowsreplay.OpenResourceBundle(
    "wowsreplay/testdata/game/15.6.0_12830008",
)
if err != nil {
    return err
}

replay, err := wowsreplay.ParseFileWithResources("battle.wowsreplay", resources)
if err != nil {
    return err
}

match := replay.BuildMatch()
```

`Match` contains entity tracks, identities, teams, health/state histories,
deaths, combat events, and weapon traces. `Match.Unknown` explicitly counts
payloads that remain lossless in the event stream but were not promoted into
the match model.

### Fog of war and multiple perspectives

A client replay only records entities and armament visible to that client.
This is a source-data limitation: no parser can recover an enemy position that
the server never transmitted. If recordings from multiple players in the same
battle are available, `MergeMatches` validates their arena identity and unions
their tracks, state, and weapon observations:

```go
merged, err := wowsreplay.MergeMatches(
    firstReplay.BuildMatch(),
    secondReplay.BuildMatch(),
)
if err != nil {
    return err
}

options.Match = merged
options.Resources = resources
err = firstReplay.RenderVideo(ctx, "merged.mp4", options)
```

`PerspectiveTeams` and `FogOfWarLimited` state whether the merged result covers
both teams. Raw packet payloads remain available regardless of semantic support.

For the next semantic-decoding layer, provide:

1. A few representative `.wowsreplay` files, ideally one from each client
   version that matters.
2. The matching extracted `scripts/entity_defs` and aliases/type definitions
   from that client build.
3. Extracted game parameters if ship names, classes, teams, consumables, health,
   damage, or capture events should be rendered semantically.
4. Optional map metadata/backgrounds if videos should use true map bounds or
   terrain instead of movement-derived bounds.

Put local replay fixtures in `wowsreplay/testdata/replays/` and matching client
data in `wowsreplay/testdata/game/<major.minor.patch>_<build>/`. These large or
potentially redistributability-sensitive files are intentionally ignored by
Git. Set `WOWS_REPLAY_TEST_FILE` to run the optional real-replay integration
test:

```powershell
$env:WOWS_REPLAY_TEST_FILE = "wowsreplay/testdata/replays/example.wowsreplay"
go test ./wowsreplay
```

The container and packet layout behavior was checked against
[landaire/wows-toolkit](https://github.com/landaire/wows-toolkit), including
legacy packet identifiers before WoWS 12.6.
