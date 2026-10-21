package api

import (
	"net/http"
	"strings"

	"uitest-server/internal/model"
)

/* ---------------------------------------------------------------- 设置 */

type aiSettingsView struct {
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	HasKey     bool   `json:"has_key"`
	KeyHint    string `json:"key_hint"`
	TimeoutSec int    `json:"timeout_sec"`
}

type settingsView struct {
	AI      aiSettingsView        `json:"ai"`
	Browser model.BrowserSettings `json:"browser"`
	Server  map[string]any        `json:"server"`
}

func (s *Server) settingsView() settingsView {
	st := s.DB.Settings()
	hint := ""
	if st.AI.APIKey != "" {
		if len(st.AI.APIKey) <= 8 {
			hint = "****"
		} else {
			hint = st.AI.APIKey[:4] + "****" + st.AI.APIKey[len(st.AI.APIKey)-4:]
		}
	}
	return settingsView{
		AI: aiSettingsView{
			BaseURL:    st.AI.BaseURL,
			Model:      st.AI.Model,
			HasKey:     st.AI.APIKey != "",
			KeyHint:    hint,
			TimeoutSec: st.AI.TimeoutSec,
		},
		Browser: st.Browser,
		Server:  map[string]any{"port": s.Port, "version": s.Version},
	}
}

// getSettings GET /api/settings
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsView())
}

type putSettingsReq struct {
	AI *struct {
		BaseURL    *string `json:"base_url"`
		APIKey     *string `json:"api_key"`
		Model      *string `json:"model"`
		TimeoutSec *int    `json:"timeout_sec"`
	} `json:"ai"`
	Browser *struct {
		Executable      *string `json:"executable"`
		Headless        *bool   `json:"headless"`
		StepDelayMS     *int    `json:"step_delay_ms"`
		ActionTimeoutMS *int    `json:"action_timeout_ms"`
		NoSandbox       *bool   `json:"no_sandbox"`
		DebugURL        *string `json:"debug_url"`
	} `json:"browser"`
}

// putSettings PUT /api/settings
// 语义：只覆盖显式提交的字段；ai.api_key 传空字符串表示保持原值。
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var req putSettingsReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	st := s.DB.Settings()
	if req.AI != nil {
		if req.AI.BaseURL != nil {
			st.AI.BaseURL = strings.TrimSpace(*req.AI.BaseURL)
		}
		if req.AI.Model != nil {
			st.AI.Model = strings.TrimSpace(*req.AI.Model)
		}
		if req.AI.APIKey != nil && strings.TrimSpace(*req.AI.APIKey) != "" {
			st.AI.APIKey = strings.TrimSpace(*req.AI.APIKey)
		}
		if req.AI.TimeoutSec != nil && *req.AI.TimeoutSec > 0 {
			st.AI.TimeoutSec = *req.AI.TimeoutSec
		}
	}
	if req.Browser != nil {
		if req.Browser.Executable != nil {
			st.Browser.Executable = strings.TrimSpace(*req.Browser.Executable)
		}
		if req.Browser.Headless != nil {
			st.Browser.Headless = *req.Browser.Headless
		}
		if req.Browser.StepDelayMS != nil && *req.Browser.StepDelayMS >= 0 {
			st.Browser.StepDelayMS = *req.Browser.StepDelayMS
		}
		if req.Browser.ActionTimeoutMS != nil && *req.Browser.ActionTimeoutMS > 0 {
			st.Browser.ActionTimeoutMS = *req.Browser.ActionTimeoutMS
		}
		if req.Browser.NoSandbox != nil {
			st.Browser.NoSandbox = *req.Browser.NoSandbox
		}
		if req.Browser.DebugURL != nil {
			st.Browser.DebugURL = strings.TrimRight(strings.TrimSpace(*req.Browser.DebugURL), "/")
		}
	}
	s.DB.SaveSettings(st)
	s.BR.ApplySettings(st.Browser)
	writeJSON(w, http.StatusOK, s.settingsView())
}

/* -------------------------------------------------------------- 浏览器 */

// browserStatus GET /api/browser/status
func (s *Server) browserStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.BR.Status())
}

// browserLaunch POST /api/browser/launch
func (s *Server) browserLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	_ = readJSON(r, &req)
	if _, err := s.BR.Ensure(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := s.BR.Status()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "debug_url": st.DebugURL, "executable": st.Executable, "version": st.Version})
}

// browserClose POST /api/browser/close
func (s *Server) browserClose(w http.ResponseWriter, r *http.Request) {
	if err := s.BR.Close(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
