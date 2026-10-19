package service

import (
	"log"
	"time"

	"kefu-server/internal/platform/realtime"
)

// ---------------- 基础设施层回调注册 ----------------
// platform 下的包不反向依赖业务内核，涉及业务逻辑的部分在这里注册回调：
//   - realtime：客服最后一个连接断开 → 释放其接待中的会话
// 注册点落在会话内核的 init 里（route 会 import 本包，保证注册被执行）。
// 另一个回调 authctx.SetUserLoader 归属账号内核，见 common/service/auth_service.go。

func init() {
	realtime.SetAgentOfflineHook(NewConversationService().releaseAgentConversations)
}

// releaseAgentConversations 把某客服正在接待的会话全部置回「待接待」。
// 用于客服（含管理员）离线后自动释放，避免会话被已离线的人长期占用。
func (s *ConversationService) releaseAgentConversations(tenantID, agentID uint) {
	convs, err := s.repo.ListOpenConversationsByAgent(tenantID, agentID)
	if err != nil || len(convs) == 0 {
		return
	}
	now := time.Now()
	for _, c := range convs {
		c.AgentID = 0
		c.Status = "pending"
		c.LastMessageAt = now
		_ = s.repo.SaveConversation(c)
		s.addSystemMessage(tenantID, c.ID, s.agentName(agentID)+" 已离线，会话回到待接待")
		vo := s.buildConversationVO(c)
		realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.updated", Data: vo})
		if m := s.lastSystemMessage(tenantID, c.ID); m != nil {
			realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "message.new", Data: *m})
		}
	}
	log.Printf("[hub] 客服 #%d 离线，释放 %d 个会话回待接待", agentID, len(convs))
}
