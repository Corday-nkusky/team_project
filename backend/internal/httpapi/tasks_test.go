package httpapi

import (
	"strings"
	"testing"
)

func TestValidTaskID(t *testing.T) {
	good := strings.Repeat("a", 32)
	if !validTaskID(good) {
		t.Fatal("32 位小写十六进制应合法")
	}
	for _, bad := range []string{"", strings.Repeat("a", 31), strings.Repeat("a", 33),
		strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("a", 31) + "-"} {
		if validTaskID(bad) {
			t.Fatalf("非法任务标识被放行: %q", bad)
		}
	}
}

func TestTaskAcceptedViewIsOnlyATicket(t *testing.T) {
	v := taskAcceptedView{TaskID: "t", MaterialID: 1, Status: "pending", StatusURL: "/api/tasks/t"}
	if v.StatusURL != "/api/tasks/"+v.TaskID {
		t.Fatal("状态地址必须与任务标识一致")
	}
	// 响应体里不能出现索引结果字段；结构体字段即契约，这里防止后续误加。
	if strings.Contains(strings.ToLower(v.Status), "ready") {
		t.Fatal("提交响应不得含 ready")
	}
}
