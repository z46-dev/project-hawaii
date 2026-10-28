package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func performRequest(url string) (body *http.Response, err error) {
	body, err = http.Get(url)
	return
}

// QueryRecentAIReplays queries the replayswows.com API for the most recent AI replays and returns the response as an APISearchResponse struct.
func QueryRecentAIReplays() (resp *APISearchResponse, err error) {
	var body *http.Response
	if body, err = performRequest("https://replayswows.com/api/search/index?by=desc&order=uploaded_at"); err != nil {
		return
	}

	var decoder *json.Decoder = json.NewDecoder(body.Body)
	err = decoder.Decode(&resp)
	return
}

// DownloadReplay downloads a .wowsreplay file from replayswows.com given a replay ID and returns a ReadCloser for the file.
func DownloadReplay(replayID uint) (replayFile *WowsReplayFile, err error) {
	var body *http.Response
	if body, err = performRequest(fmt.Sprintf("https://replayswows.com/replay/download/%d", replayID)); err != nil {
		return
	}

	replayFile = NewWowsReplayFile(replayID, body.Body)
	return
}
