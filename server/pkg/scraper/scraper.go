package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/z46-dev/golog"
)

// performRequest performs an HTTP GET request to the specified URL and returns the response body or an error if the request fails or returns a non-200 status code.
func performRequest(url string) (body *http.Response, err error) {
	if body, err = http.Get(url); err == nil && body.StatusCode != http.StatusOK {
		_ = body.Body.Close()
		err = fmt.Errorf("unexpected status code: %d", body.StatusCode)
	}

	return
}

// performAPIRequest performs an HTTP GET request to the replayswows.com API with the specified query parameters and returns the response as an APISearchResponse struct or an error if the request fails or returns a non-200 status code.
func performAPIRequest(parameters string) (resp *APISearchResponse, err error) {
	var body *http.Response
	if body, err = performRequest(fmt.Sprintf("https://replayswows.com/api/search/index?%s", parameters)); err != nil {
		return
	}

	defer body.Body.Close()

	var decoder *json.Decoder = json.NewDecoder(body.Body)
	err = decoder.Decode(&resp)
	return
}

// QueryRecentAPIReplays queries the replayswows.com API for the most recent AI replays and returns the response as an APISearchResponse struct.
func QueryRecentAPIReplays() (resp *APISearchResponse, err error) {
	resp, err = performAPIRequest("by=desc&order=uploaded_at")
	return
}

// QueryAPIReplaysByPage queries the replayswows.com API for AI replays on a specific page and returns the response as an APISearchResponse struct.
func QueryAPIReplaysByPage(page uint) (resp *APISearchResponse, err error) {
	resp, err = performAPIRequest(fmt.Sprintf("by=desc&order=uploaded_at&page=%d", page))
	return
}

// QueryAPIReplaysByCount queries the replayswows.com API for a specific number of AI replays and returns the response as a series of APISearchResponseReplay structs.
func QueryAPIReplaysByCount(count int) (replays []*APISearchResponseReplay, err error) {
	var page uint = 1

	for len(replays) < count {
		var pageResp *APISearchResponse
		if pageResp, err = QueryAPIReplaysByPage(page); err != nil {
			return
		}

		replays = append(replays, pageResp.Replays...)
		if page++; pageResp.Pages == nil || pageResp.Pages.Current >= pageResp.Pages.Total {
			break
		}
	}

	replays = replays[:min(len(replays), count)]
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

// DownloadReplays downloads multiple .wowsreplay files from replayswows.com given a list of replay IDs and returns a slice of WowsReplayFile structs. The downloads are performed concurrently using the specified number of workers.
func DownloadReplays(replayIDs []uint, numWorkers uint, printProgressLogger *golog.Logger) (replayFiles []*WowsReplayFile, err error) {
	numWorkers = min(max(numWorkers, 1), uint(len(replayIDs)))

	var (
		total                       int = len(replayIDs)
		wg                          sync.WaitGroup
		consumeMu, produceMu, errMu sync.Mutex
		loader                      *golog.Loader
	)

	if printProgressLogger != nil {
		loader = printProgressLogger.Loader(fmt.Sprintf("Downloading replays 0/%d", len(replayIDs)), golog.LoaderASCII, 5)
		loader.Start()
		defer loader.Stop()
	}

	for range numWorkers {
		wg.Go(func() {
			var (
				consumeReplayID func() (id uint, ok bool) = func() (id uint, ok bool) {
					consumeMu.Lock()
					defer consumeMu.Unlock()

					if len(replayIDs) == 0 {
						id, ok = 0, false
					} else {
						id, replayIDs, ok = replayIDs[0], replayIDs[1:], true
					}

					return
				}
				produceReplayFile func(f *WowsReplayFile) = func(f *WowsReplayFile) {
					produceMu.Lock()
					defer produceMu.Unlock()

					replayFiles = append(replayFiles, f)
					if loader != nil {
						var done int = len(replayFiles)
						loader.SetProgress(float64(done) / float64(total))
						loader.SetMessage(fmt.Sprintf("Downloading replays %d/%d", done, total))
					}
				}
				id       uint
				ok       bool
				localErr error
			)

			for {
				if id, ok = consumeReplayID(); !ok {
					return
				}

				var replayFile *WowsReplayFile
				if replayFile, localErr = DownloadReplay(id); localErr != nil {
					errMu.Lock()
					defer errMu.Unlock()

					if err == nil {
						err = fmt.Errorf("failed to download replay ID %d: %+v", id, localErr)
					}

					return
				}

				wg.Go(func() {
					produceReplayFile(replayFile)
				})
			}
		})
	}

	wg.Wait()

	if err != nil {
		for _, f := range replayFiles {
			_ = f.Close()
		}

		replayFiles = nil
	}

	return
}
