package httpapi

import (
	"sync"
	"time"
)

const materialCacheTTL = 2 * time.Second

type materialCacheEntry struct {
	expires time.Time
	items   []materialView
}

// materialListCache 是按班级隔离的短 TTL 读缓存；不缓存错误，写入路径主动失效。
// worker 在独立进程中更新索引状态，因此状态变化最迟在 TTL 到期后可见。
type materialListCache struct {
	mu      sync.RWMutex
	entries map[int64]materialCacheEntry
	ttl     time.Duration
	now     func() time.Time
}

func newMaterialListCache(ttl time.Duration) *materialListCache {
	return &materialListCache{entries: make(map[int64]materialCacheEntry), ttl: ttl, now: time.Now}
}

func cloneMaterialViews(items []materialView) []materialView {
	return append([]materialView(nil), items...)
}

func (c *materialListCache) get(classID int64) ([]materialView, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[classID]
	c.mu.RUnlock()
	if !ok || !c.now().Before(entry.expires) {
		return nil, false
	}
	return cloneMaterialViews(entry.items), true
}

func (c *materialListCache) set(classID int64, items []materialView) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[int64]materialCacheEntry)
	}
	c.entries[classID] = materialCacheEntry{expires: c.now().Add(c.ttl), items: cloneMaterialViews(items)}
}

func (c *materialListCache) invalidate(classID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, classID)
	c.mu.Unlock()
}
