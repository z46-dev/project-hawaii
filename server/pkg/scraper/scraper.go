package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
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

// commonConsumer is a generic function that safely consumes an item from a slice of items using a mutex for synchronization.
// It returns the consumed item and a boolean indicating whether an item was successfully consumed.
func commonConsumer[T any](items *[]T, mu *sync.Mutex) (item T, ok bool) {
	mu.Lock()
	defer mu.Unlock()

	if len(*items) == 0 {
		item, ok = *new(T), false
	} else {
		item, *items, ok = (*items)[0], (*items)[1:], true
	}

	return
}

// commonProducer is a generic function that safely appends an item to a slice of items using a mutex for synchronization.
func commonProducer[T any](item T, items *[]T, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	*items = append(*items, item)
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
				id       uint
				ok       bool
				localErr error
			)

			for {
				if id, ok = commonConsumer(&replayIDs, &consumeMu); !ok {
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
					commonProducer(replayFile, &replayFiles, &produceMu)
					if loader != nil {
						var done int = len(replayFiles)
						loader.SetProgress(float64(done) / float64(total))
						loader.SetMessage(fmt.Sprintf("Downloading replays %d/%d", done, total))
					}
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

// SaveReplays saves multiple .wowsreplay files to the specified directory.
func SaveReplays(replays []*WowsReplayFile, directory string, numWorkers uint, name func(id uint) (fileName string), printProgressLogger *golog.Logger) (err error) {
	numWorkers = min(max(numWorkers, 1), uint(len(replays)))

	if name == nil {
		name = func(id uint) (fileName string) {
			fileName = fmt.Sprintf("replayswows_com_%d.wowsreplay", id)
			return
		}
	}

	var (
		total            int = len(replays)
		wg               sync.WaitGroup
		consumeMu, errMu sync.Mutex
		loader           *golog.Loader
	)

	if printProgressLogger != nil {
		loader = printProgressLogger.Loader(fmt.Sprintf("Saving replays 0/%d", len(replays)), golog.LoaderASCII, 5)
		loader.Start()
		defer loader.Stop()
	}

	// Ensure directory
	if err = os.MkdirAll(directory, 0755); err != nil {
		return
	}

	for range numWorkers {
		wg.Go(func() {
			var (
				replay   *WowsReplayFile
				ok       bool
				localErr error
			)

			for {
				if replay, ok = commonConsumer(&replays, &consumeMu); !ok {
					return
				}

				if localErr = replay.SaveTo(path.Join(directory, name(replay.ID))); localErr != nil {
					errMu.Lock()
					defer errMu.Unlock()

					if err == nil {
						err = fmt.Errorf("failed to save replay ID %d: %+v", replay.ID, localErr)
					}

					return
				}

				wg.Go(func() {
					if loader != nil {
						var done int = total - len(replays)
						loader.SetProgress(float64(done) / float64(total))
						loader.SetMessage(fmt.Sprintf("Saving replays %d/%d", done, total))
					}
				})
			}
		})
	}

	wg.Wait()
	return
}
