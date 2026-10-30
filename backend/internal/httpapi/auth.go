package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResp struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	User         userView `json:"user"`
}

type userView struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	ClassID     int64  `json:"class_id"`
	ClassName   string `json:"class_name"`
	DisplayName string `json:"display_name"`
}

func viewOf(u store.User) userView {
	return userView{
		ID: u.ID, Username: u.Username, Role: u.Role,
		ClassID: u.ClassID, ClassName: u.ClassName, DisplayName: u.DisplayName,
	}
}

// newTokens 签发一对新令牌，并把 refresh 的摘要入库（只存摘要，design D8）。
func (s *Server) newTokens(ctx context.Context, u store.User) (tokenResp, error) {
	access, err := auth.SignAccess(s.cfg.JWTSecret, auth.Claims{
		UserID: u.ID, Role: u.Role, ClassID: u.ClassID, Name: u.DisplayName,
	}, s.cfg.AccessTTL)
	if err != nil {
		return tokenResp{}, err
	}
	plain, hash, err := auth.NewRefreshToken()
	if err != nil {
		return tokenResp{}, err
	}
	if err := s.st.SaveRefreshToken(ctx, u.ID, hash, time.Now().Add(s.cfg.RefreshTTL)); err != nil {
		return tokenResp{}, err
	}
	return tokenResp{
		AccessToken:  access,
		RefreshToken: plain,
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		User:         viewOf(u),
	}, nil
}

// handleLogin 处理 POST /api/login（R1.1 / R1.2 / R1.3）。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeErr(w, err, "请求体格式不正确")
		return
	}
	ctx := r.Context()

	// 账号不存在与口令错误走同一条失败路径，响应体完全一致（R1.2）。
	fail := func(reason string) {
		_ = s.st.Audit(ctx, nil, req.Username, nil, "login_failure", "fail", reason)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "账号或口令不正确")
	}

	u, err := s.st.GetUserByUsername(ctx, req.Username)
	if errors.Is(err, store.ErrNotFound) {
		fail("账号不存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "服务内部错误")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		fail("口令错误")
		return
	}

	resp, err := s.newTokens(ctx, u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "签发凭证失败")
		return
	}
	uid, cid := u.ID, u.ClassID
	_ = s.st.Audit(ctx, &uid, u.Username, &cid, "login_success", "ok", "角色:"+u.Role)
	writeJSON(w, http.StatusOK, resp)
}

// handleRefresh 处理 POST /api/refresh：以 refresh 换新 access，并轮换 refresh（R1.6 / R1.7）。
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.RefreshToken == "" {
		// 解析失败与凭证无效同形，避免通过错误码探测令牌格式（R1.2 / R11）。
		writeDecodeErrAs(w, err, http.StatusUnauthorized, "invalid_refresh", "刷新凭证无效")
		return
	}
	ctx := r.Context()

	userID, err := s.st.ConsumeRefreshToken(ctx, auth.HashToken(payload.RefreshToken))
	if err != nil { // 不存在 / 过期 / 已登出 / 已轮换，一律 401（R1.7）
		writeErr(w, http.StatusUnauthorized, "invalid_refresh", "刷新凭证无效，请重新登录")
		return
	}
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_refresh", "刷新凭证无效，请重新登录")
		return
	}
	resp, err := s.newTokens(ctx, u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "签发凭证失败")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLogout 处理 POST /api/logout：作废 refresh token（R1.5）。
//
// 不要求 access 仍有效——只要持有 refresh 即可作废，避免「access 过期就登不出去」。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeErr(w, err, "请求体格式不正确")
		return
	}
	if payload.RefreshToken != "" {
		_ = s.st.RevokeRefreshToken(r.Context(), auth.HashToken(payload.RefreshToken))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// handleMe 返回当前登录者信息，供前端决定界面。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "未认证")
		return
	}
	u, err := s.st.GetUserByID(r.Context(), c.UserID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, viewOf(u))
}
