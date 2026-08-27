package lava

import (
	"encoding/json"
	"testing"
)

func TestSearchIdentifier(t *testing.T) {
	cases := map[string]string{
		"never gonna give you up": "ytsearch:never gonna give you up",
		"https://youtu.be/x":      "https://youtu.be/x",
		"http://example.com/a":    "http://example.com/a",
		"ytsearch:explicit":       "ytsearch:explicit",
	}
	for in, want := range cases {
		if got := SearchIdentifier(in); got != want {
			t.Errorf("SearchIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	good := map[string]int64{"90": 90, "1:30": 90, "1:02:03": 3723, "0:05": 5}
	for in, want := range good {
		if n, ok := ParseTimestamp(in); !ok || n != want {
			t.Errorf("ParseTimestamp(%q) = %d, %v; want %d", in, n, ok, want)
		}
	}
	for _, bad := range []string{"", "x", "-4", "1:2:3:4", "1:x"} {
		if _, ok := ParseTimestamp(bad); ok {
			t.Errorf("ParseTimestamp(%q) should fail", bad)
		}
	}
}

func TestFormatMillis(t *testing.T) {
	cases := map[int64]string{
		0:       "0:00",
		90000:   "1:30",
		-5:      "0:00",
		3723000: "1:02:03",
	}
	for in, want := range cases {
		if got := FormatMillis(in); got != want {
			t.Errorf("FormatMillis(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadResultParsing(t *testing.T) {
	var raw struct {
		LoadType string          `json:"loadType"`
		Data     json.RawMessage `json:"data"`
	}

	raw.LoadType = "SEARCH_RESULT"
	raw.Data = json.RawMessage(`[{"encoded":"abc","info":{"title":"Song","length":95000,"isSeekable":true,"author":"A"}}]`)
	b, _ := json.Marshal(raw)
	var res LoadResultWire
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseLoad(res.LoadType, res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.AllTracks()) != 1 || parsed.AllTracks()[0].Info.Title != "Song" {
		t.Fatalf("search parse wrong: %+v", parsed)
	}

	raw.LoadType = "PLAYLIST_LOADED"
	raw.Data = json.RawMessage(`{"info":{"name":"Mix","selectedTrack":1},"tracks":[{"encoded":"a"},{"encoded":"b"}]}`)
	b, _ = json.Marshal(raw)
	res = LoadResultWire{}
	_ = json.Unmarshal(b, &res)
	parsed, err = ParseLoad(res.LoadType, res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Playlist == nil || len(parsed.AllTracks()) != 2 || parsed.Playlist.Info.Name != "Mix" {
		t.Fatalf("playlist parse wrong: %+v", parsed)
	}

	raw.LoadType = "NO_MATCHES"
	raw.Data = json.RawMessage(`[]`)
	b, _ = json.Marshal(raw)
	res = LoadResultWire{}
	_ = json.Unmarshal(b, &res)
	parsed, err = ParseLoad(res.LoadType, res.Data)
	if err != nil || len(parsed.AllTracks()) != 0 {
		t.Fatalf("no-matches should be empty, got %+v err %v", parsed, err)
	}
}

func TestQueueOperations(t *testing.T) {
	q := &Queue{}
	mk := func(s string) QueuedTrack { return QueuedTrack{Track: Track{Encoded: s}} }

	if _, ok := q.Pop(); ok {
		t.Fatal("empty pop should miss")
	}
	q.Add(mk("a"))
	q.Add(mk("b"))
	q.Add(mk("c"))
	if q.Len() != 3 || q.Pages(10) != 1 || q.Pages(2) != 2 {
		t.Fatalf("len/pages wrong: %d %d %d", q.Len(), q.Pages(10), q.Pages(2))
	}
	if p := q.Page(3, 2); p != nil {
		t.Fatal("past-end page should be nil")
	}
	if p := q.Page(2, 2); len(p) != 1 || p[0].Track.Encoded != "c" {
		t.Fatalf("page 2 wrong: %+v", p)
	}

	q.AddNext(mk("top"))
	if t1, _ := q.Pop(); t1.Track.Encoded != "top" {
		t.Fatalf("AddNext should land first, got %q", t1.Track.Encoded)
	}
	if !q.Remove(1) || q.Remove(9) {
		t.Fatal("remove semantics wrong")
	}
	if q.Len() != 2 {
		t.Fatalf("want 2 left, got %d", q.Len())
	}
	q.Shuffle()
	q.Clear()
	if q.Len() != 0 {
		t.Fatal("clear failed")
	}
}

type LoadResultWire struct {
	LoadType string          `json:"loadType"`
	Data     json.RawMessage `json:"data"`
}

