// Command e2e 是本项目的端到端验收脚本：驱动真实 HTTP 接口 + 直连数据库核对，
// 覆盖 spec 的 R1–R7 与权限负向用例。**退出码即判定**：全绿为 0，任一失败为 1。
//
// 用法（服务已在运行）：
//
//	cd backend && go run ./cmd/e2e
//
// 环境变量与服务端一致（MYSQL_*、E2E_BASE 可选）。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/db"
)

var (
	base   = envOr("E2E_BASE", "http://127.0.0.1:8080")
	passed int
	failed int
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	conn, err := db.Open(cfg.DSN())
	if err != nil {
		fmt.Println("无法连接数据库:", err)
		os.Exit(1)
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		fmt.Println("数据库不可用:", err)
		os.Exit(1)
	}

	fmt.Printf("== CampusClaw 端到端验收 ==  base=%s\n\n", base)

	// 两个「独有词」必须彼此无词面重叠，否则会互相命中。数字后缀用相隔很远的取值，
	// 避免两次取模恰好相同（这会让跨班用例失去意义）。
	n := time.Now().UnixNano() % 1_000_000
	unique := fmt.Sprintf("量子纠缠实验码%d", n)
	uniqueB := fmt.Sprintf("跨班密语%d", (n+500_003)%1_000_000)

	checkHealth(ctx)
	teacher := checkLoginAndTokens(ctx)
	checkLoginFailureUniform(ctx)
	checkAuthGuards(ctx, teacher.access)

	// 令牌轮换 / 登出需要独立的会话，避免互相干扰。
	checkRefreshRotation(ctx)
	checkLogout(ctx)

	student1 := login(ctx, "student1", "student123")
	teacher2 := login(ctx, "teacher2", "teacher123")

	checkStudentCannotUpload(ctx, student1.access)

	// 上传与检索（R3.1 / R5.1 / R6 / R7）。
	matID := checkUpload(ctx, teacher.access, unique)
	checkUploadEmpty(ctx, teacher.access)
	checkReindex(ctx, teacher.access, matID)
	askHits(ctx, student1.access, unique)
	checkClientAbort(ctx, student1.access, unique)

	// 跨班负向（R2.1 / R2.2）。
	mat2 := uploadMaterial(ctx, teacher2.access, "B班材料", uniqueB) // 仅存在于 2 班
	checkCrossClassReindex(ctx, teacher.access, mat2)
	checkCrossClassSearch(ctx, student1.access, uniqueB)

	checkAskEmpty(ctx)
	askNoEvidence(ctx, student1.access)
	checkAudit(ctx, unique)

	fmt.Printf("\n== 合计: %d 通过, %d 失败 ==\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// --- 各组检查 ---

func checkHealth(ctx context.Context) {
	sec("地基：探活与部署（tasks 1.4）")
	st, body := get(ctx, "/health", "")
	ok("GET /health 返回 200", st == 200, fmt.Sprintf("实际 %d", st))
	var h struct {
		Deps map[string]string `json:"deps"`
	}
	_ = json.Unmarshal(body, &h)
	ok("依赖 mysql / qdrant 均就绪", h.Deps["mysql"] == "ok" && h.Deps["qdrant"] == "ok", string(body))
}

func checkLoginAndTokens(ctx context.Context) session {
	sec("R1 账号口令认证与登录态（R1.1 / R1.2）")
	s := login(ctx, "teacher1", "teacher123")
	ok("正确凭据登录返回 200", s.status == 200, fmt.Sprintf("实际 %d", s.status))
	ok("响应含 access_token 与 refresh_token", s.access != "" && s.refresh != "", "")
	ok("响应不含口令相关字段", !strings.Contains(strings.ToLower(s.raw), "password"), "")
	return s
}

func checkLoginFailureUniform(ctx context.Context) {
	sec("R1.2 失败响应同形")
	stA, bodyA := post(ctx, "/api/login", "", map[string]string{"username": "no_such_user_xyz", "password": "whatever"})
	stB, bodyB := post(ctx, "/api/login", "", map[string]string{"username": "teacher1", "password": "wrong-password"})
	ok("账号不存在返回 401", stA == 401, fmt.Sprintf("实际 %d", stA))
	ok("口令错误返回 401", stB == 401, fmt.Sprintf("实际 %d", stB))
	ok("两者状态码与响应体完全一致（不可区分）", stA == stB && string(bodyA) == string(bodyB), fmt.Sprintf("%s vs %s", bodyA, bodyB))
}

func checkAuthGuards(ctx context.Context, access string) {
	sec("R1.3 / R1.4 凭证校验")
	tampered := tamper(access)
	st, _ := get(ctx, "/api/me", tampered)
	ok("篡改一字节的 access token → 401", st == 401, fmt.Sprintf("实际 %d", st))
	st2, _ := get(ctx, "/api/me", "")
	ok("未携带 Authorization → 401", st2 == 401, fmt.Sprintf("实际 %d", st2))
}

func checkRefreshRotation(ctx context.Context) {
	sec("R1.6 / R1.7 refresh 轮换")
	s := login(ctx, "student1", "student123")
	st, body := post(ctx, "/api/refresh", "", map[string]string{"refresh_token": s.refresh})
	var refreshed tokenPayload
	_ = json.Unmarshal(body, &refreshed)
	ok("以有效 refresh 换取新 access（200）", st == 200 && refreshed.AccessToken != "", fmt.Sprintf("status=%d", st))
	ok("同时返回新的 refresh_token（轮换）", refreshed.RefreshToken != "" && refreshed.RefreshToken != s.refresh, "")
	st2, _ := post(ctx, "/api/refresh", "", map[string]string{"refresh_token": s.refresh})
	ok("旧的 refresh 立即失效 → 401", st2 == 401, fmt.Sprintf("实际 %d", st2))
}

func checkLogout(ctx context.Context) {
	sec("R1.5 登出使 refresh 失效")
	s := login(ctx, "student1", "student123")
	st, _ := post(ctx, "/api/logout", "", map[string]string{"refresh_token": s.refresh})
	ok("登出返回 200", st == 200, fmt.Sprintf("实际 %d", st))
	st2, _ := post(ctx, "/api/refresh", "", map[string]string{"refresh_token": s.refresh})
	ok("登出后该 refresh 无法再换取 access → 401", st2 == 401, fmt.Sprintf("实际 %d", st2))
	st3, _ := get(ctx, "/api/me", s.access)
	ok("已签发的 access 在有效期内仍可用（已知取舍）", st3 == 200, fmt.Sprintf("实际 %d", st3))
}

func checkStudentCannotUpload(ctx context.Context, studentAccess string) {
	sec("R2.3 学生不能写材料")
	st, _ := post(ctx, "/api/materials", studentAccess, map[string]string{"title": "x", "content": "y"})
	ok("学生调用上传接口 → 403", st == 403, fmt.Sprintf("实际 %d", st))
}

func checkUpload(ctx context.Context, teacherAccess, unique string) int64 {
	sec("R3.1 教师上传成功并可检索")
	content := fmt.Sprintf("本节讲义讨论 %s 的构造与验证方法。它是本材料的独有词，仅用于检索验证。", unique)
	st, body := post(ctx, "/api/materials", teacherAccess, map[string]string{"title": "讲义甲", "content": content})
	var mv materialPayload
	_ = json.Unmarshal(body, &mv)
	ok("教师上传非空材料 → 201", st == 201, fmt.Sprintf("实际 %d: %s", st, body))
	ok("索引状态为 ready", mv.IndexStatus == "ready", mv.IndexStatus+mv.IndexError)
	return mv.ID
}

func checkUploadEmpty(ctx context.Context, teacherAccess string) {
	sec("R3.2 空材料被拒")
	st, _ := post(ctx, "/api/materials", teacherAccess, map[string]string{"title": "空", "content": "   \n  "})
	ok("内容为空 → 400", st == 400, fmt.Sprintf("实际 %d", st))
}

func checkReindex(ctx context.Context, teacherAccess string, id int64) {
	sec("R3.4 重索引")
	st, body := post(ctx, fmt.Sprintf("/api/materials/%d/reindex", id), teacherAccess, nil)
	var mv materialPayload
	_ = json.Unmarshal(body, &mv)
	ok("对材料重索引 → 200 且状态 ready", st == 200 && mv.IndexStatus == "ready", fmt.Sprintf("status=%d body=%s", st, body))
}

func askHits(ctx context.Context, access, unique string) {
	sec("R5.1 / R6 / R7 命中检索 → 流式带引用回答")
	events := askSSE(ctx, access, unique+" 的构造方法是什么？")
	ok("响应 Content-Type 为 text/event-stream", events.contentType == "text/event-stream", events.contentType)
	ok("收到 meta 事件且含非空引用清单", len(events.citations) > 0, fmt.Sprintf("%d 条", len(events.citations)))
	ok("引用来自本班材料（有材料标题）", firstTitle(events.citations) != "", "")
	ok("收到至少一个 delta 增量", events.deltaCount > 0, fmt.Sprintf("%d 个", events.deltaCount))
	ok("收到明确的 done 结束事件", events.done, "")
	ok("回答文本非空", strings.TrimSpace(events.text) != "", "")
}

// checkClientAbort 覆盖 R6.3：客户端在流式过程中断开，服务端应停止生成、
// 留「客户端中断」痕迹，且不落半条问答记录。
func checkClientAbort(ctx context.Context, access, unique string) {
	sec("R6.3 客户端中途断开")
	cfg := config.Load()
	conn, _ := db.Open(cfg.DSN())
	defer conn.Close()

	countAudit := func() int {
		var n int
		_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='ask' AND detail='客户端中断'`).Scan(&n)
		return n
	}
	countConv := func() int {
		var n int
		_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations`).Scan(&n)
		return n
	}
	beforeAudit, beforeConv := countAudit(), countConv()

	// 可取消请求：读到首个 delta 后立即取消，模拟客户端中途掉线。
	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/api/ask",
		bytes.NewReader(mustJSON(map[string]string{"question": unique + " 的构造方法是什么？"})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)

	if resp, err := http.DefaultClient.Do(req); err == nil {
		br := bufio.NewReader(resp.Body)
		for {
			line, rerr := br.ReadString('\n')
			if rerr != nil {
				break
			}
			if strings.HasPrefix(line, "data:") && strings.Contains(line, "\"text\"") {
				break // 已开始出增量，此刻断开
			}
		}
		cancel()
		_ = resp.Body.Close()
	}
	time.Sleep(600 * time.Millisecond) // 等服务端察觉断开并留痕

	ok("断开后服务端留「客户端中断」痕迹", countAudit() > beforeAudit,
		fmt.Sprintf("%d → %d", beforeAudit, countAudit()))
	ok("断开不留半条问答记录（不落库）", countConv() == beforeConv,
		fmt.Sprintf("%d → %d", beforeConv, countConv()))
}

func askNoEvidence(ctx context.Context, access string) {
	sec("R5.2 / R7.3 未命中 → 固定文案 + no_evidence")
	events := askSSE(ctx, access, "请解释一个本班材料完全没有涉及的主题："+fmt.Sprint(time.Now().UnixNano()))
	ok("标记 no_evidence", events.noEvidence, "")
	ok("回答为固定文案「资料中未找到相关内容」", strings.Contains(events.text, "资料中未找到相关内容"), events.text)
	ok("引用清单为空（不伪造引用）", len(events.citations) == 0, fmt.Sprintf("%d 条", len(events.citations)))
}

func checkAskEmpty(ctx context.Context) {
	sec("R5.3 空问题被拒")
	st, _ := post(ctx, "/api/ask", login(ctx, "student1", "student123").access, map[string]string{"question": "   "})
	ok("空白问题 → 400", st == 400, fmt.Sprintf("实际 %d", st))
}

func uploadMaterial(ctx context.Context, access, title, unique string) int64 {
	st, body := post(ctx, "/api/materials", access, map[string]string{"title": title, "content": "只属于另一个班的材料，独有词：" + unique})
	var mv materialPayload
	_ = json.Unmarshal(body, &mv)
	if st != 201 {
		fmt.Printf("    (准备 B 班材料失败: %d %s)\n", st, body)
	}
	return mv.ID
}

func checkCrossClassReindex(ctx context.Context, teacher1Access string, class2MaterialID int64) {
	sec("R2.1 跨班读被拒")
	st, body := post(ctx, fmt.Sprintf("/api/materials/%d/reindex", class2MaterialID), teacher1Access, nil)
	ok("A 班教师请求 B 班材料 → 404/403 且不含 B 内容", (st == 404 || st == 403) && !strings.Contains(string(body), "跨班密语"), fmt.Sprintf("status=%d body=%s", st, body))
	// 列表也应只含本班材料
	_, listBody := get(ctx, "/api/materials", teacher1Access)
	ok("材料列表不含他班材料", !strings.Contains(string(listBody), "跨班密语"), "")
}

func checkCrossClassSearch(ctx context.Context, student1Access, uniqueB string) {
	sec("R2.2 跨班检索不泄漏")
	events := askSSE(ctx, student1Access, uniqueB)
	leaked := strings.Contains(events.text, uniqueB)
	detail := fmt.Sprintf("leaked=%v noEvidence=%v cites=%d ct=%q text=%q",
		leaked, events.noEvidence, len(events.citations), events.contentType, events.text)
	if len(events.citations) > 0 {
		detail += fmt.Sprintf(" firstCite=%q", events.citations[0].MaterialTitle)
	}
	ok("只在 B 班存在的内容，A 班检索不到（无引用且不出现该词）",
		!leaked && len(events.citations) == 0, detail)
}

func checkAudit(ctx context.Context, unique string) {
	sec("R4 操作留痕")
	cfg := config.Load()
	conn, _ := db.Open(cfg.DSN())
	defer conn.Close()

	count := func(action string) int {
		var n int
		_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action=?`, action).Scan(&n)
		return n
	}
	ok("登录成功有留痕", count("login_success") > 0, "")
	ok("登录失败有留痕", count("login_failure") > 0, "")
	ok("材料上传有留痕", count("upload") > 0, "")
	ok("问答有留痕", count("ask") > 0, "")

	var detail string
	_ = conn.QueryRowContext(ctx, `SELECT detail FROM audit_logs WHERE action='login_failure' ORDER BY id DESC LIMIT 1`).Scan(&detail)
	ok("失败登录留痕不含明文口令", !strings.Contains(strings.ToLower(detail), "password") && !strings.Contains(detail, "wrong"), detail)
}

// --- 基础设施 ---

type session struct {
	status  int
	access  string
	refresh string
	raw     string
}

type tokenPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type materialPayload struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	IndexStatus string `json:"index_status"`
	IndexError  string `json:"index_error"`
}

type sseResult struct {
	contentType string
	text        string
	citations   []struct {
		Index         int    `json:"index"`
		MaterialTitle string `json:"material_title"`
	}
	noEvidence bool
	deltaCount int
	done       bool
}

func login(ctx context.Context, user, pass string) session {
	st, body := post(ctx, "/api/login", "", map[string]string{"username": user, "password": pass})
	var p tokenPayload
	_ = json.Unmarshal(body, &p)
	return session{status: st, access: p.AccessToken, refresh: p.RefreshToken, raw: string(body)}
}

func askSSE(ctx context.Context, access, question string) sseResult {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/ask",
		bytes.NewReader(mustJSON(map[string]string{"question": question})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sseResult{}
	}
	defer resp.Body.Close()

	res := sseResult{contentType: strings.Split(resp.Header.Get("Content-Type"), ";")[0]}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch event {
		case "meta":
			var m struct {
				NoEvidence bool `json:"no_evidence"`
				Citations  []struct {
					Index         int    `json:"index"`
					MaterialTitle string `json:"material_title"`
				} `json:"citations"`
			}
			if json.Unmarshal([]byte(data), &m) == nil {
				res.noEvidence = m.NoEvidence
				res.citations = m.Citations
			}
		case "delta":
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(data), &d) == nil {
				res.text += d.Text
				res.deltaCount++
			}
		case "done":
			res.done = true
		}
	}
	return res
}

func get(ctx context.Context, path, token string) (int, []byte) {
	return req(ctx, http.MethodGet, path, token, nil)
}

func post(ctx context.Context, path, token string, body any) (int, []byte) {
	return req(ctx, http.MethodPost, path, token, body)
}

func req(ctx context.Context, method, path, token string, body any) (int, []byte) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(mustJSON(body))
	}
	r, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
	if err != nil {
		return 0, nil
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func tamper(token string) string {
	if token == "" {
		return "x.y.z"
	}
	b := []byte(token)
	// 改动签名段首字符
	parts := strings.Split(string(b), ".")
	if len(parts) == 3 && len(parts[2]) > 0 {
		c := parts[2][0]
		if c == 'A' {
			parts[2] = "B" + parts[2][1:]
		} else {
			parts[2] = "A" + parts[2][1:]
		}
		return strings.Join(parts, ".")
	}
	return token + "x"
}

func firstTitle(cs []struct {
	Index         int    `json:"index"`
	MaterialTitle string `json:"material_title"`
}) string {
	if len(cs) == 0 {
		return ""
	}
	return cs[0].MaterialTitle
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// --- 输出 ---

func sec(title string) { fmt.Printf("\n[%s]\n", title) }

func ok(name string, cond bool, detail string) {
	if cond {
		passed++
		fmt.Printf("  \033[32m✓\033[0m %s\n", name)
		return
	}
	failed++
	fmt.Printf("  \033[31m✗\033[0m %s  %s\n", name, detail)
}
