package httpapi

import (
	"sync"
	"testing"
	"time"
)

func TestMaterialCacheClassIsolationExpiryAndInvalidation(t *testing.T) {
	cache := newMaterialListCache(materialCacheTTL)
	now := time.Now()
	cache.now = func() time.Time { return now }
	cache.set(1, []materialView{{ID: 11, Title: "一班材料"}})
	cache.set(2, []materialView{{ID: 22, Title: "二班材料"}})

	items, ok := cache.get(1)
	if !ok || len(items) != 1 || items[0].ID != 11 {
		t.Fatalf("一班缓存读取错误：%+v ok=%v", items, ok)
	}
	items[0].Title = "调用方修改"
	again, _ := cache.get(1)
	if again[0].Title != "一班材料" {
		t.Fatal("缓存不得把内部切片暴露给调用方修改")
	}
	cache.invalidate(1)
	if _, ok := cache.get(1); ok {
		t.Fatal("上传/重索引后一班缓存必须失效")
	}
	if items, ok := cache.get(2); !ok || items[0].ID != 22 {
		t.Fatal("一班失效不得误清二班缓存")
	}
	now = now.Add(materialCacheTTL + time.Millisecond)
	if _, ok := cache.get(2); ok {
		t.Fatal("worker 更新状态所依赖的 TTL 到期后必须重新查询")
	}
}

func TestMaterialCacheDisabled(t *testing.T) {
	cache := newMaterialListCache(0)
	cache.set(1, []materialView{{ID: 11}})
	if _, ok := cache.get(1); ok {
		t.Fatal("TTL=0 的 A/B 基线必须完全绕过缓存")
	}
}

func TestMaterialCacheConcurrentAccess(t *testing.T) {
	cache := newMaterialListCache(materialCacheTTL)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				classID := int64(worker%2 + 1)
				cache.set(classID, []materialView{{ID: int64(i)}})
				cache.get(classID)
				if i%10 == 0 {
					cache.invalidate(classID)
				}
			}
		}(worker)
	}
	wg.Wait()
}
