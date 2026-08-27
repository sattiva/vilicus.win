package discord

import (
	"sync"
	"time"
)


const (
	snipeTTL     = 5 * time.Minute
	editCacheCap = 5000
	editCacheTTL = 10 * time.Minute
)

type snipeEntry struct {
	content  string
	authorID string
	at       time.Time
}

type snipeStore struct {
	mu sync.Mutex
	m  map[string]snipeEntry
}

func newSnipeStore() *snipeStore {
	return &snipeStore{m: make(map[string]snipeEntry)}
}

func (s *snipeStore) set(channelID, content, authorID string) {
	s.mu.Lock()
	s.m[channelID] = snipeEntry{content: content, authorID: authorID, at: time.Now()}
	s.mu.Unlock()
}

func (s *snipeStore) latest(channelID string) (snipeEntry, bool) {
	s.mu.Lock()
	e, ok := s.m[channelID]
	s.mu.Unlock()
	if !ok || time.Since(e.at) > snipeTTL {
		if ok {
			s.mu.Lock()
			delete(s.m, channelID)
			s.mu.Unlock()
		}
		return snipeEntry{}, false
	}
	return e, true
}

type editEntry struct {
	content string
	at      time.Time
}

type editCache struct {
	mu sync.Mutex
	m  map[string]editEntry
}

func newEditCache() *editCache {
	return &editCache{m: make(map[string]editEntry)}
}

func (c *editCache) set(msgID, content string) {
	c.mu.Lock()
	if len(c.m) >= editCacheCap {
		cut := time.Now().Add(-editCacheTTL)
		for k, v := range c.m {
			if v.at.Before(cut) {
				delete(c.m, k)
			}
		}
		for k := range c.m {
			if len(c.m) < editCacheCap {
				break
			}
			delete(c.m, k)
		}
	}
	c.m[msgID] = editEntry{content: content, at: time.Now()}
	c.mu.Unlock()
}

func (c *editCache) get(msgID string) (string, bool) {
	c.mu.Lock()
	e, ok := c.m[msgID]
	c.mu.Unlock()
	if !ok || time.Since(e.at) > editCacheTTL {
		return "", false
	}
	return e.content, true
}

