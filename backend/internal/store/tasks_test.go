package store

import (
	"strings"
	"testing"
)

func TestContentHashIsDerivedFromBusinessInput(t *testing.T) {
	a := ContentHash("讲义", "正文")
	b := ContentHash("讲义", "正文")
	if a != b || len(a) != 64 {
		t.Fatalf("同一标题与正文必须得到稳定的 64 位摘要: %q / %q", a, b)
	}
	if ContentHash("讲义", "正文变了") == a || ContentHash("别的标题", "正文") == a {
		t.Fatal("标题或正文变化后摘要必须变化")
	}
}

func TestIdempotencyKeysAreStableAndScoped(t *testing.T) {
	hash := ContentHash("讲义", "正文")
	k1 := UploadIdempotencyKey(1, hash)
	if k1 != UploadIdempotencyKey(1, hash) {
		t.Fatal("同一班同一内容必须得到同一个幂等键")
	}
	if k1 == UploadIdempotencyKey(2, hash) {
		t.Fatal("不同班级不得共用幂等键（班级隔离）")
	}
	if k1 == UploadIdempotencyKey(1, ContentHash("讲义", "另一份正文")) {
		t.Fatal("内容变化后幂等键必须变化")
	}

	digest := ContentDigest("正文")
	r1 := ReindexIdempotencyKey(7, digest, 0)
	if r1 != ReindexIdempotencyKey(7, digest, 0) {
		t.Fatal("重复重索引点击必须命中同一个键")
	}
	if r1 == ReindexIdempotencyKey(7, digest, 1) {
		t.Fatal("索引成功后 revision+1，应允许对同一内容再次重建")
	}
	if len(r1) > 191 || len(k1) > 191 {
		t.Fatal("幂等键超出 VARCHAR(191) 上限")
	}
}

func TestClipError(t *testing.T) {
	long := strings.Repeat("错", maxTaskErrorRune+50)
	if got := clipError(long); len([]rune(got)) != maxTaskErrorRune {
		t.Fatalf("错误原因应截断到 %d 个字符，得到 %d", maxTaskErrorRune, len([]rune(got)))
	}
	if clipError("短错误") != "短错误" {
		t.Fatal("短错误不应被改写")
	}
}
