package httpapi

// 解题引导技能的配置接口（spec SK1）。读=登录，写=教师；跨班一律 404。

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Corday-nkusky/team_project/backend/internal/mcpclient"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type skillView struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Enabled     bool     `json:"enabled"`
	Instruction string   `json:"instruction"`
	ToolRefs    []string `json:"tool_refs"`
	Version     int      `json:"version"`
	UpdatedAt   string   `json:"updated_at"`
}

func toSkillView(s store.Skill) skillView {
	return skillView{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Enabled: s.Enabled,
		Instruction: s.Instruction, ToolRefs: s.ToolRefs, Version: s.Version, UpdatedAt: s.UpdatedAt,
	}
}

type skillReq struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Enabled     *bool    `json:"enabled"`
	Instruction string   `json:"instruction"`
	ToolRefs    []string `json:"tool_refs"`
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	list, err := s.skillsRW.ListSkills(r.Context(), c.ClassID)
	if err != nil {
		logServerError(r, "list_skills", err)
		writeErr(w, http.StatusInternalServerError, "internal", "读取技能失败")
		return
	}
	out := make([]skillView, 0, len(list))
	for _, sk := range list {
		out = append(out, toSkillView(sk))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleUpsertSkill(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req skillReq
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeErr(w, err, "请求体格式不正确")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 64 {
		writeErr(w, http.StatusBadRequest, "bad_request", "技能名称必须为 1–64 字符")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "solve_hint"
	}
	if kind != "solve_hint" {
		writeErr(w, http.StatusBadRequest, "bad_request", "当前仅支持 solve_hint 类技能")
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" || len([]rune(instruction)) > 2000 {
		writeErr(w, http.StatusBadRequest, "bad_request", "引导提示词必须为 1–2000 字符")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.ToolRefs == nil {
		req.ToolRefs = []string{}
	}

	// 校验工具引用：格式 + 工具在发现清单内 + 服务器已连接（spec MC3.2）。
	if len(req.ToolRefs) > 0 {
		toolsByServer, err := s.connectedToolsByServer(r, c.ClassID)
		if err != nil {
			logServerError(r, "check_tool_refs", err)
			writeErr(w, http.StatusInternalServerError, "internal", "校验工具引用失败")
			return
		}
		if err := mcpclient.ValidateRefs(req.ToolRefs, toolsByServer); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_tool_ref", err.Error())
			return
		}
	}

	// 已指定 id 时，确认是本班技能（跨班 404）；否则按 name 新建/更新。
	if req.ID > 0 {
		if _, err := s.skillsRW.GetSkillByID(r.Context(), c.ClassID, req.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not_found", "技能不存在")
				return
			}
			logServerError(r, "get_skill", err)
			writeErr(w, http.StatusInternalServerError, "internal", "读取技能失败")
			return
		}
	}

	saved, err := s.skillsRW.UpsertSkill(r.Context(), store.Skill{
		ClassID: c.ClassID, OwnerID: c.UserID, Name: req.Name, Kind: kind,
		Enabled: enabled, Instruction: instruction, ToolRefs: req.ToolRefs,
	})
	if err != nil {
		logServerError(r, "upsert_skill", err)
		writeErr(w, http.StatusInternalServerError, "internal", "保存技能失败")
		return
	}
	if err := s.rec.Audit(r.Context(), &c.UserID, c.Name, &c.ClassID, "skill_upsert", "ok",
		saved.Name+" v"+strconv.Itoa(saved.Version)); err != nil {
		logServerError(r, "audit_skill", err)
	}
	writeJSON(w, http.StatusOK, toSkillView(saved))
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "技能 id 不合法")
		return
	}
	if err := s.skillsRW.DeleteSkill(r.Context(), c.ClassID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", "技能不存在")
			return
		}
		logServerError(r, "delete_skill", err)
		writeErr(w, http.StatusInternalServerError, "internal", "删除技能失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// connectedToolsByServer 汇总本班已连接服务器的工具清单，供引用校验使用。
func (s *Server) connectedToolsByServer(r *http.Request, classID int64) (map[string][]mcpclient.Tool, error) {
	servers, err := s.mcp.ListMCPServers(r.Context(), classID)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]mcpclient.Tool)
	for _, sv := range servers {
		if !sv.Connected {
			continue
		}
		for _, t := range sv.Tools {
			out[sv.Name] = append(out[sv.Name], mcpclient.Tool{
				Name: t.Name, Description: t.Description, InputSchema: t.InputSchema,
			})
		}
	}
	return out, nil
}
