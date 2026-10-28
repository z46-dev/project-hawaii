package scraper

import (
	"io"
	"os"
)

// NewWowsReplayFile creates a new WowsReplayFile struct with the given ID and ReadCloser.
func NewWowsReplayFile(id uint, body io.ReadCloser) (f *WowsReplayFile) {
	f = &WowsReplayFile{
		ID:         id,
		ReadCloser: body,
	}

	return
}

// SaveTo saves the WowsReplayFile to the specified path.
func (f *WowsReplayFile) SaveTo(path string) (err error) {
	var file *os.File

	if file, err = os.Create(path); err != nil {
		return
	}

	defer file.Close()

	_, err = io.Copy(file, f.ReadCloser)
	return
}
