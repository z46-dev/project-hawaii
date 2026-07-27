/*
Package wowsreplay parses World of Warships .wowsreplay files without starting
the game client.

The parser accepts paths, byte slices, or any io.Reader:

	var replay *wowsreplay.Replay
	var err error

	replay, err = wowsreplay.ParseFile("battle.wowsreplay")
	replay, err = wowsreplay.Parse(bytes.NewReader(upload))

Every packet is represented by an Event. Packet types whose layouts are stable
across game versions expose typed Data. Entity methods and properties require
matching game-version definitions; this package preserves their complete raw
payloads so a definition decoder can be added without reparsing the container.

Event logs can be written as JSON Lines or compact text. WriteVideo and
RenderVideo create a simple tactical-map MP4 and require an ffmpeg executable
with libx264 support.
*/
package wowsreplay
