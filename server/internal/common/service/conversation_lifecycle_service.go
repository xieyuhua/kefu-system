package service

import (
	"errors"
	"log"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/storage"
)

// FindOrCreateConversation 定位或创建会话。
//
// 核心约定：同一个 uid + 同一渠道 = 同一个客户，同一个客户 = **始终复用最近一个会话**。
// 即使该会话已「结束」（closed）或被「归档」（archived），也会自动重新激活为「待接待」，
// 从而保证聊天记录**自然合并**，不会因为客服关闭 / 归档而分裂成多个会话。
//
// 供 GuestInit / GuestSend / WebSocket 访客连接三处统一使用，避免各自写不一致的过滤条件。
func (s *ConversationService) FindOrCreateConversation(tenantID, customerID, channelID uint) (*model.Conversation, error) {
	if conv, err := s.repo.FindLatestConversationByCustomer(tenantID, customerID); err == nil {
		// 复用最近一个会话；已关闭 / 已归档的重新激活
		needSave := false
		if conv.Status == "closed" {
			conv.ClosedAt = nil
			// 有归属人 → 回到「进行中」，保持由他接待。
			//
			// 这里刻意**不判断该客服是否在线**：客户发消息不该成为「释放归属」的触发器。
			// 之前判在线导致「管理员刚接待、随后用户发消息就变未接待」——只要那一刻
			// 客服的 WebSocket 刚好不在（关了工作台标签、换设备、网络抖动），
			// 会话就被判回待接待，看起来就像「接待被撤销了」。
			// 真正的人走茶凉交给离线钩子处理（断连 90s 后统一释放）。
			if conv.AgentID > 0 {
				conv.Status = "open"
			} else {
				conv.Status = "pending"
			}
			needSave = true
		}
		if conv.Archived {
			conv.Archived = false
			conv.ArchivedAt = nil
			conv.Status = "pending"
			conv.AgentID = 0 // 归档本来就会释放归属，这里保证一致性
			needSave = true
		}
		if needSave {
			if err := s.repo.SaveConversation(conv); err != nil {
				return nil, err
			}
		}
		return conv, nil
	}

	// 首次咨询：新建会话
	now := time.Now()
	conv := &model.Conversation{
		TenantID: tenantID, ChannelID: channelID, CustomerID: customerID,
		Status: "pending", LastMessageAt: now,
	}
	if err := s.repo.CreateConversation(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// ---------------- 会话归档 / 删除 ----------------

// ArchiveConversation 归档会话（客服主动清理工作台列表）。
// 仅隐藏，不删除数据；客户再次发消息会自动恢复（见 ReopenConversation）。
func (s *ConversationService) ArchiveConversation(tenantID, convID, operatorID uint) error {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return errors.New("会话不存在")
	}
	now := time.Now()
	updates := map[string]interface{}{
		"archived":    true,
		"archived_at": &now,
		"unread":      0,
	}
	// 归档时若该客服不再接待，释放归属（重新进入待接待池）
	if c.AgentID == operatorID {
		updates["agent_id"] = 0
		updates["status"] = "pending"
	}
	return s.repo.UpdateConversationByID(c.ID, updates)
}

// ArchiveConversationsBulk 批量归档会话（一键清空工作台列表）。
// 复用单条归档语义：仅隐藏不删数据，客户再次咨询会自动恢复。
// q 使用与列表查询完全相同的过滤条件，保证「所见即所清」。
func (s *ConversationService) ArchiveConversationsBulk(tenantID, userID uint, isAdmin bool, q ConvQuery) (int, error) {
	// 分批归档：单页上限受 storage.MaxPageSize 约束（防 DoS 的全局夹取），
	// 归档后的会话不再出现在可见列表里，因此每批都从第 1 页取，直到取空或达到批数上限，
	// 避免「一键清空」在大数据量下只处理第一批就返回。
	const maxBatches = 100 // 单次最多处理 100*MaxPageSize 条，防止误操作无限循环
	total := 0
	for batch := 0; batch < maxBatches; batch++ {
		q.Page = 1
		q.PageSize = storage.MaxPageSize

		paged, err := s.ListConversations(tenantID, userID, isAdmin, q)
		if err != nil {
			return total, err
		}
		// ListConversations 返回的 List 是 []interface{}，逐项取出会话 ID。
		items, ok := paged.List.([]interface{})
		if !ok || len(items) == 0 {
			break
		}

		ids := make([]uint, 0, len(items))
		for _, it := range items {
			switch v := it.(type) {
			case map[string]interface{}:
				// VO 以 map 形态进入（JSON 往返）
				if f, ok := v["id"].(float64); ok {
					ids = append(ids, uint(f))
				}
			case dto.ConversationVO:
				ids = append(ids, v.ID)
			}
		}
		if len(ids) == 0 {
			log.Printf("[bulk] 未从列表解析出会话 id，items=%d", len(items))
			break
		}

		n, err := s.archiveConversationIDs(tenantID, userID, ids)
		total += n
		if err != nil {
			return total, err
		}
		if len(items) < q.PageSize {
			break // 已取到最后一页
		}
	}
	return total, nil
}

// archiveConversationIDs 归档一批会话：先查归属以判断是否需要释放回待接待池。
func (s *ConversationService) archiveConversationIDs(tenantID, userID uint, ids []uint) (int, error) {
	now := time.Now()
	rows, err := s.repo.ListConversationAgentRefs(tenantID, ids)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, c := range rows {
		updates := map[string]interface{}{
			"archived":    true,
			"archived_at": &now,
			"unread":      0,
		}
		// 归档时若该客服不再接待，释放归属（重新进入待接待池）
		if c.AgentID == userID {
			updates["agent_id"] = 0
			updates["status"] = "pending"
		}
		if err := s.repo.UpdateConversationByID(c.ID, updates); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// UnarchiveConversation 取消归档，恢复到工作台列表
func (s *ConversationService) UnarchiveConversation(tenantID, convID uint) error {
	return s.repo.SetConversationUnarchived(tenantID, convID)
}

// DeleteConversation 彻底删除会话及其消息（不可恢复）
func (s *ConversationService) DeleteConversation(tenantID, convID uint) error {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return errors.New("会话不存在")
	}
	return s.repo.DeleteConversationWithMessages(tenantID, c.ID)
}

// ReopenConversation 客户重新发起咨询时调用：
// 若会话被归档，则取消归档并回到「未接待」状态（消息不丢）。
func (s *ConversationService) ReopenConversation(tenantID, convID uint) {
	// 仅命中已归档的会话，重复调用无副作用；失败不阻断（与原逻辑一致）
	_ = s.repo.ReopenArchivedConversation(tenantID, convID)
}

// ---------------- 客户在线状态（活跃度推断） ----------------

// onlineWindow 活跃判定窗口：超过此时长无活动视为离线（微信式）
const onlineWindow = 2 * time.Minute

// CustomerOnlineWindow 对外暴露的活跃判定窗口（端内编排在标记访客离线时要据此把
// last_seen_at 推到窗口之外，避免两处各写一个魔法值导致口径漂移）。
const CustomerOnlineWindow = onlineWindow

// IsCustomerOnline 按最近活跃时间推断客户是否在线
func IsCustomerOnline(c *model.Customer) bool {
	if c.LastSeenAt == nil {
		return false
	}
	return time.Since(*c.LastSeenAt) <= onlineWindow
}
