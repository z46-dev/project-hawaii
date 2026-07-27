package wowsreplay

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

type (
	// LogFormat selects the serialization used by WriteEventLog.
	LogFormat string

	// LogOptions controls event log formatting and raw payload inclusion.
	LogOptions struct {
		Format         LogFormat
		IncludePayload bool
	}
)

const (
	// LogFormatJSONLines writes one machine-readable JSON object per line.
	LogFormatJSONLines LogFormat = "jsonl"

	// LogFormatText writes one compact human-readable event per line.
	LogFormatText LogFormat = "text"
)

// WriteEventLog writes every replay event in chronological order.
func (replay *Replay) WriteEventLog(writer io.Writer, options LogOptions) (err error) {
	var bufferedWriter *bufio.Writer = bufio.NewWriter(writer)

	if options.Format == "" {
		options.Format = LogFormatJSONLines
	}

	switch options.Format {
	case LogFormatJSONLines:
		err = replay.writeJSONLines(bufferedWriter, options.IncludePayload)
	case LogFormatText:
		err = replay.writeTextLog(bufferedWriter, options.IncludePayload)
	default:
		err = fmt.Errorf("unsupported event log format %q", options.Format)
	}

	if err == nil {
		err = bufferedWriter.Flush()
	}

	return
}

// writeJSONLines writes a metadata record followed by one JSON record per event.
func (replay *Replay) writeJSONLines(writer io.Writer, includePayload bool) (err error) {
	var encoder *json.Encoder = json.NewEncoder(writer)

	if err = encoder.Encode(map[string]any{
		"record":   "metadata",
		"metadata": replay.Metadata,
		"stats":    replay.Stats,
		"warnings": replay.Warnings,
	}); err != nil {
		return
	}

	for _, event := range replay.Events {
		var output Event = event

		if !includePayload {
			output.RawPayload = nil
		}

		if err = encoder.Encode(struct {
			Record string `json:"record"`
			Event
		}{
			Record: "event",
			Event:  output,
		}); err != nil {
			return
		}
	}

	return
}

// writeTextLog writes compact event summaries and decoded data.
func (replay *Replay) writeTextLog(writer io.Writer, includePayload bool) (err error) {
	var encoded []byte

	if _, err = fmt.Fprintf(
		writer,
		"# WoWS replay: %s | %s | %s | %d packets\n",
		replay.Metadata.PlayerName,
		replay.Metadata.MapDisplayName,
		replay.Metadata.ClientVersionFromExe,
		len(replay.Events),
	); err != nil {
		return
	}

	for _, event := range replay.Events {
		if event.Data != nil {
			if encoded, err = json.Marshal(event.Data); err != nil {
				return
			}

			if _, err = fmt.Fprintf(writer, "%s %s", event.String(), encoded); err != nil {
				return
			}
		} else if _, err = fmt.Fprint(writer, event.String()); err != nil {
			return
		}

		if includePayload {
			if _, err = fmt.Fprintf(writer, " raw=%s", hex.EncodeToString(event.RawPayload)); err != nil {
				return
			}
		}

		if _, err = fmt.Fprintln(writer); err != nil {
			return
		}
	}

	return
}
