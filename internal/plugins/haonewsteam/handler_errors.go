package haonewsteam

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	teamcore "hao.news/internal/haonews/team"
)

type teamErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Help    string `json:"help,omitempty"`
	DocURL  string `json:"doc_url,omitempty"`
}

func writeTeamAPIError(w http.ResponseWriter, status int, resp teamErrorResponse) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func classifyTeamAPIError(teamID string, err error) (teamErrorResponse, bool) {
	if err == nil {
		return teamErrorResponse{}, false
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "signature verification failed"):
		return teamErrorResponse{
			Error:   "message_signature_required",
			Message: "本 Team 要求消息签名，当前消息未通过签名校验。",
			Help:    "请在消息里携带有效的 Ed25519 签名；如果只是网页快捷发送，请先查看 Team Policy 再改用带签名的 API 请求。",
			DocURL:  "/api/teams/" + teamID + "/policy",
		}, true
	case errors.Is(err, teamcore.ErrForbidden):
		return teamErrorResponse{
			Error:   "permission_denied",
			Message: "当前角色没有执行这个 Team 动作的权限。",
			Help:    "请检查 Team Policy 中的角色权限设置，或改用具备权限的成员身份执行。",
			DocURL:  "/api/teams/" + teamID + "/policy",
		}, true
	case errors.Is(err, teamcore.ErrInvalidState):
		return teamErrorResponse{
			Error:   "invalid_task_transition",
			Message: "当前任务状态不能按这个方向流转。",
			Help:    "请先查看 Team 的任务流转规则，再选择允许的下一个状态。",
			DocURL:  "/api/teams/" + teamID + "/policy",
		}, true
	default:
		return teamErrorResponse{}, false
	}
}
