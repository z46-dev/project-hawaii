package scraper

import "io"

type (
	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponse struct {
		Replays []*APISearchResponseReplay `json:"replays"`
		SQL     string                     `json:"sql"`
		Pages   *APISearchResponsePages    `json:"pages"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplay struct {
		ID         uint                             `json:"id"`
		Title      string                           `json:"title"`
		PlayerName string                           `json:"playerName"`
		Ship       *APISearchResponseReplayShip     `json:"ship"`
		Result     *APISearchResponseReplayResult   `json:"result"`
		IsKraken   bool                             `json:"isKraken"`
		Stats      *APISearchResponseReplayStats    `json:"stats"`
		Scenario   *APISearchResponseReplayScenario `json:"scenario"`
		Version    string                           `json:"version"`
		Video      *string                          `json:"video"`
		Links      *APISearchResponseReplayLinks    `json:"_links"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayShip struct {
		Name       string                            `json:"name"`
		NameIDS    string                            `json:"name_ids"`
		Nation     string                            `json:"nation"`
		Image      string                            `json:"image"`
		ImageLarge string                            `json:"image_large"`
		Level      *APISearchResponseReplayShipLevel `json:"level"`
		Type       string                            `json:"type"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayResult struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayStats struct {
		Damage int `json:"damage"`
		Frags  int `json:"frags"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayScenario struct {
		MapName  string `json:"mapName"`
		ModeName string `json:"modeName"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayLinks struct {
		Self    *APISearchResponseReplayLinksSelf    `json:"self"`
		Filters *APISearchResponseReplayLinksFilters `json:"filters"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayLinksSelf struct {
		Href string `json:"href"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayLinksFilters struct {
		Map        *APISearchResponseReplayLinksFiltersMap        `json:"map"`
		BattleType *APISearchResponseReplayLinksFiltersBattleType `json:"battle_type"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayLinksFiltersMap struct {
		Href string `json:"href"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayLinksFiltersBattleType struct {
		Href string `json:"href"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponseReplayShipLevel struct {
		Arabic int    `json:"arabic"`
		Roman  string `json:"roman"`
	}

	// https://replayswows.com/api/search/index?by=desc&order=uploaded_at
	APISearchResponsePages struct {
		Total   int `json:"total"`
		Current int `json:"current"`
		Count   int `json:"count"`
	}

	WowsReplayFile struct {
		ID uint
		io.ReadCloser
	}
)
