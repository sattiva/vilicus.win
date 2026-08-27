package lava

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

func ParseLoad(loadType string, data json.RawMessage) (*LoadResult, error) {
	res := &LoadResult{LoadType: loadType}
	switch loadType {
	case "TRACK_LOADED", "SEARCH_RESULT":
		var tracks []Track
		if err := json.Unmarshal(data, &tracks); err != nil {
			return nil, err
		}
		res.Tracks = tracks
	case "PLAYLIST_LOADED":
		var pl PlaylistData
		if err := json.Unmarshal(data, &pl); err != nil {
			return nil, err
		}
		res.Playlist = &pl
	}
	return res, nil
}


type TrackInfo struct {
	Identifier string `json:"identifier"`
	IsSeekable bool   `json:"isSeekable"`
	Author     string `json:"author"`
	Length     int64  `json:"length"`
	IsStream   bool   `json:"isStream"`
	Position   int64  `json:"position"`
	Title      string `json:"title"`
	URI        string `json:"uri"`
	SourceName string `json:"sourceName"`
}

type Track struct {
	Encoded string    `json:"encoded"`
	Info    TrackInfo `json:"info"`
}

type PlaylistData struct {
	Info struct {
		Name          string `json:"name"`
		SelectedTrack int    `json:"selectedTrack"`
	} `json:"info"`
	Tracks []Track `json:"tracks"`
}

type LoadResult struct {
	LoadType string
	Tracks   []Track
	Playlist *PlaylistData
}

func (r *LoadResult) AllTracks() []Track {
	if r == nil {
		return nil
	}
	if r.Playlist != nil {
		return r.Playlist.Tracks
	}
	return r.Tracks
}

type ServerMessage struct {
	Op        string `json:"op"`
	SessionID string `json:"sessionId,omitempty"`
	GuildID   string `json:"guildId,omitempty"`
	Type      string `json:"type,omitempty"`
	State     *struct {
		Time      int64 `json:"time"`
		Position  int64 `json:"position"`
		Connected bool  `json:"connected"`
	} `json:"state,omitempty"`

	Reason   string `json:"reason,omitempty"`
	Code     int    `json:"code,omitempty"`
	ByRemote bool   `json:"byRemote,omitempty"`
}

type PlayerPatch struct {
	Track    *EncodedTrack `json:"track,omitempty"`
	Volume   *int          `json:"volume,omitempty"`
	Paused   *bool         `json:"paused,omitempty"`
	Position *int64        `json:"position,omitempty"`
	Voice    *VoiceFields  `json:"voice,omitempty"`
}

type EncodedTrack struct {
	Encoded string `json:"encoded"`
}

type VoiceFields struct {
	Token     string `json:"token"`
	Endpoint  string `json:"endpoint"`
	SessionID string `json:"sessionId"`
}

func SearchIdentifier(query string) string {
	if strings.HasPrefix(strings.ToLower(query), "http://") || strings.HasPrefix(strings.ToLower(query), "https://") {
		return query
	}
	if _, err := url.ParseRequestURI(query); err == nil && strings.Contains(query, ":") {
		return query
	}
	return "ytsearch:" + query
}

func ParseTimestamp(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	total := int64(0)
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 || len(p) > 2 {
			return 0, false
		}
		total = total*60 + n
	}
	return total, true
}

func FormatMillis(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	sec := ms / 1000
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return strconv.FormatInt(h, 10) + ":" + pad2(m) + ":" + pad2(s)
	}
	return strconv.FormatInt(m, 10) + ":" + pad2(s)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

func randomID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

