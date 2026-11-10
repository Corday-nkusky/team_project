package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectionJSON(size int) string {
	return fmt.Sprintf(`{"result":{"config":{"params":{"vectors":{"size":%d,"distance":"Cosine"}}}},"status":"ok"}`, size)
}

func TestEnsureCollectionAcceptsMatchingDimension(t *testing.T) {
	put := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			put = true
			return
		}
		_, _ = w.Write([]byte(collectionJSON(2048)))
	}))
	defer srv.Close()

	if err := New(srv.URL, "").EnsureCollection(context.Background(), "chunks", 2048); err != nil {
		t.Fatalf("维度一致应成功: %v", err)
	}
	if put {
		t.Fatal("集合已存在且维度一致时不应再次创建")
	}
}

// 换嵌入模型后维度不一致必须在删除旧向量前失败（LG2.3 / design D4）。
func TestEnsureCollectionRejectsDimensionMismatch(t *testing.T) {
	put := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			put = true
			return
		}
		_, _ = w.Write([]byte(collectionJSON(1024)))
	}))
	defer srv.Close()

	err := New(srv.URL, "").EnsureCollection(context.Background(), "chunks", 2048)
	if err == nil || !strings.Contains(err.Error(), "1024") || !strings.Contains(err.Error(), "2048") {
		t.Fatalf("维度不一致应报错并列出两个维度，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "删除该集合") {
		t.Fatalf("错误应给出运维动作，得到 %v", err)
	}
	if put {
		t.Fatal("维度不一致时不得覆盖或重建集合")
	}
}

func TestEnsureCollectionCreatesWhenMissing(t *testing.T) {
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/collections/chunks" {
			t.Fatalf("意外的创建请求: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Fatalf("解析创建请求: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	if err := New(srv.URL, "").EnsureCollection(context.Background(), "chunks", 2048); err != nil {
		t.Fatalf("集合不存在时应创建: %v", err)
	}
	vectors, _ := created["vectors"].(map[string]any)
	if vectors["size"] != float64(2048) || vectors["distance"] != "Cosine" {
		t.Fatalf("创建参数不符: %v", created)
	}
}

func TestEnsureCollectionReportsQueryFailure(t *testing.T) {
	put := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			put = true
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	err := New(srv.URL, "").EnsureCollection(context.Background(), "chunks", 2048)
	if err == nil || !strings.Contains(err.Error(), "查询集合") {
		t.Fatalf("查询失败应明确报错，得到 %v", err)
	}
	if put {
		t.Fatal("查询失败时不应盲目重建集合")
	}
}

func TestUpsertBatchesPoints(t *testing.T) {
	var batches []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Points []Point `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("解析写入请求: %v", err)
		}
		batches = append(batches, len(body.Points))
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	points := make([]Point, upsertBatchSize+72)
	for i := range points {
		points[i] = Point{ID: i + 1, Vector: []float32{1, 2}}
	}
	if err := New(srv.URL, "").Upsert(context.Background(), "chunks", points); err != nil {
		t.Fatalf("分批写入应成功: %v", err)
	}
	if len(batches) != 2 || batches[0] != upsertBatchSize || batches[1] != 72 {
		t.Fatalf("分批大小不符: %v", batches)
	}
}

func TestUpsertStopsOnBatchFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	points := make([]Point, upsertBatchSize*2)
	for i := range points {
		points[i] = Point{ID: i + 1, Vector: []float32{1, 2}}
	}
	err := New(srv.URL, "").Upsert(context.Background(), "chunks", points)
	if err == nil || !strings.Contains(err.Error(), "写入第") {
		t.Fatalf("任一批失败应整体报错，得到 %v", err)
	}
	if calls != 2 {
		t.Fatalf("失败后不应继续写后续批次，实际调用 %d 次", calls)
	}
}
