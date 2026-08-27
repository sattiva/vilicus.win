package lava

import "math/rand"

type Queue struct {
	items []QueuedTrack
}

type QueuedTrack struct {
	Track       Track
	RequesterID string
}

func (q *Queue) Len() int { return len(q.items) }

func (q *Queue) Add(t QueuedTrack) {
	q.items = append(q.items, t)
}

func (q *Queue) AddNext(t QueuedTrack) {
	q.items = append(q.items, QueuedTrack{})
	copy(q.items[1:], q.items)
	q.items[0] = t
}

func (q *Queue) Pop() (QueuedTrack, bool) {
	if len(q.items) == 0 {
		return QueuedTrack{}, false
	}
	t := q.items[0]
	q.items = q.items[1:]
	return t, true
}

func (q *Queue) Clear() { q.items = nil }

func (q *Queue) Shuffle() {
	rand.Shuffle(len(q.items), func(i, j int) { q.items[i], q.items[j] = q.items[j], q.items[i] })
}

func (q *Queue) Remove(pos int) bool {
	if pos < 0 || pos >= len(q.items) {
		return false
	}
	q.items = append(q.items[:pos], q.items[pos+1:]...)
	return true
}

func (q *Queue) Page(page, size int) []QueuedTrack {
	if size <= 0 || page < 1 || len(q.items) == 0 {
		return nil
	}
	start := (page - 1) * size
	if start >= len(q.items) {
		return nil
	}
	end := start + size
	if end > len(q.items) {
		end = len(q.items)
	}
	out := make([]QueuedTrack, end-start)
	copy(out, q.items[start:end])
	return out
}

func (q *Queue) Pages(size int) int {
	if size <= 0 || len(q.items) == 0 {
		return 0
	}
	return (len(q.items) + size - 1) / size
}

