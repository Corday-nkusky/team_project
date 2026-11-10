package qa

import "testing"

func TestPointIDDeterministic(t *testing.T) {
	a := PointID(42, 3, "异步任务的幂等键", "讲义 > 第三节")
	b := PointID(42, 3, "异步任务的幂等键", "讲义 > 第三节")
	if a != b {
		t.Fatalf("同一份切片必须得到同一点 ID: %d != %d", a, b)
	}
	if a == 0 || a > maxPointID {
		t.Fatalf("点 ID 越界: %d", a)
	}
}

func TestPointIDChangesWithContentOrPosition(t *testing.T) {
	base := PointID(42, 3, "正文", "路径")
	for name, got := range map[string]uint64{
		"正文变化": PointID(42, 3, "正文变了", "路径"),
		"路径变化": PointID(42, 3, "正文", "新路径"),
		"序号变化": PointID(42, 4, "正文", "路径"),
		"材料变化": PointID(43, 3, "正文", "路径"),
	} {
		if got == base {
			t.Fatalf("%s 不应与基线的点 ID 相同", name)
		}
	}
}
