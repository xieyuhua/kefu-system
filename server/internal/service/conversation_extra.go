package service

import (
	"errors"
	"log"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

// FindOrCreateConversation 定位或创建会话。
//
// 核心约定：同一个 uid + 同一渠道 = 同一个客户，同一个客户 = **始终复用最近一个会话**。
// 即使该会话已「结束」（closed）或被「归档」（archived），也会自动重新激活为「待接待」，
// 从而保证聊天记录**自然合并**，不会因为客服关闭 / 归档而分裂成多个会话。
//
// 供 GuestInit / GuestSend / WebSocket 访客连接三处统一使用，避免各自写不一致的过滤条件。
func FindOrCreateConversation(tenantID, customerID, channelID uint) (*model.Conversation, error) {
	if conv, err := repository.FindLatestConversationByCustomer(tenantID, customerID); err == nil {
		// 复用最近一个会话；已关闭 / 已归档的重新激活
		needSave := false
		if conv.Status == "closed" {
			conv.Status = "pending"
			conv.ClosedAt = nil
			needSave = true
		}
		if conv.Archived {
			conv.Archived = false
			conv.ArchivedAt = nil
			conv.Status = "pending"
			needSave = true
		}
		if needSave {
			if err := repository.SaveConversation(conv); err != nil {
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
	if err := repository.CreateConversation(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// ---------------- 会话归档 / 删除 ----------------

// ArchiveConversation 归档会话（客服主动清理工作台列表）。
// 仅隐藏，不删除数据；客户再次发消息会自动恢复（见 UnarchiveOnNewMessage）。
func ArchiveConversation(tenantID, convID, operatorID uint) error {
	c, err := repository.FindConversationByID(tenantID, convID)
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
	return repository.UpdateConversationByID(c.ID, updates)
}

// ArchiveConversationsBulk 批量归档会话（一键清空工作台列表）。
// 复用单条归档语义：仅隐藏不删数据，客户再次咨询会自动恢复。
// q 使用与列表查询完全相同的过滤条件，保证「所见即所清」。
func ArchiveConversationsBulk(tenantID, userID uint, isAdmin bool, q ConvQuery) (int, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	// 清空时不按分页截断，但设上限防止误操作拖垮服务
	q.PageSize = 500

	paged, err := ListConversations(tenantID, userID, isAdmin, q)
	if err != nil {
		return 0, err
	}
	// ListConversations 返回的 List 是 []interface{}，逐项取出会话 ID。
	items, ok := paged.List.([]interface{})
	if !ok || len(items) == 0 {
		return 0, nil
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
		return 0, nil
	}

	now := time.Now()
	// 先查出归属，用于判断哪些需要释放回待接待池
	rows, err := repository.ListConversationAgentRefs(tenantID, ids)
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
		if err := repository.UpdateConversationByID(c.ID, updates); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// UnarchiveConversation 取消归档，恢复到工作台列表
func UnarchiveConversation(tenantID, convID uint) error {
	return repository.UnarchiveConversation(tenantID, convID)
}

// DeleteConversation 彻底删除会话及其消息（不可恢复）
func DeleteConversation(tenantID, convID uint) error {
	c, err := repository.FindConversationByID(tenantID, convID)
	if err != nil {
		return errors.New("会话不存在")
	}
	return repository.DeleteConversationWithMessages(tenantID, c.ID)
}

// ReopenConversation 客户重新发起咨询时调用：
// 若会话被归档，则取消归档并回到「未接待」状态（消息不丢）。
func ReopenConversation(tenantID, convID uint) {
	// 仅命中已归档的会话，重复调用无副作用；失败不阻断（与原逻辑一致）
	_ = repository.ReopenArchivedConversation(tenantID, convID)
}

// ---------------- 客户在线状态（活跃度推断） ----------------

// onlineWindow 活跃判定窗口：超过此时长无活动视为离线（微信式）
const onlineWindow = 2 * time.Minute

// IsCustomerOnline 按最近活跃时间推断客户是否在线
func IsCustomerOnline(c *model.Customer) bool {
	if c.LastSeenAt == nil {
		return false
	}
	return time.Since(*c.LastSeenAt) <= onlineWindow
}

// ---------------- 消息搜索 ----------------

// SearchMessages 在指定会话内按内容关键词搜索消息，返回升序结果
func SearchMessages(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, errors.New("请输入搜索关键词")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// repository 内部按 id 倒序取最近 limit 条后反转为升序，与消息流顺序一致
	return repository.SearchMessagesByContent(tenantID, convID, kw, limit)
}

// ---------------- 全局对话记录查询（管理后台） ----------------

// ConversationRecord 对话记录查询项
type ConversationRecord struct {
	ID             uint   `json:"id"`
	CustomerID     uint   `json:"customer_id"`
	CustomerName   string `json:"customer_name"`
	CustomerAvatar string `json:"customer_avatar"`
	ChannelID      uint   `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	ChannelType    string `json:"channel_type"`
	AgentID        uint   `json:"agent_id"`
	AgentName      string `json:"agent_name"`
	Status         string `json:"status"`
	Archived       bool   `json:"archived"`
	MessageCount   int    `json:"message_count"`
	LastMessage    string `json:"last_message"`
	LastMessageAt  string `json:"last_message_at"`
	CreatedAt      string `json:"created_at"`
	FirstMessage   string `json:"first_message"`
	FirstMessageAt string `json:"first_message_at"`
}

// ConversationRecordsQuery 对话记录查询条件
type ConversationRecordsQuery struct {
	Keyword   string // 搜昵称 / 消息内容
	ChannelID uint
	Status    string
	AgentID   uint
	Start     string // YYYY-MM-DD
	End       string // YYYY-MM-DD
	Page      int
	PageSize  int
}

// SearchConversations 全局查询对话记录（支持按消息内容全文匹配）
func SearchConversations(tenantID uint, q ConversationRecordsQuery) ([]ConversationRecord, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 100 {
		q.PageSize = 20
	}
	f := repository.ConversationRecordFilter{
		Keyword:   q.Keyword,
		ChannelID: q.ChannelID,
		Status:    q.Status,
		AgentID:   q.AgentID,
		Page:      q.Page,
		PageSize:  q.PageSize,
	}
	// 日期按天解析：起始含当天零点，结束取次日零点做开区间，避免漏掉当天最后一秒
	if q.Start != "" {
		if t, err := time.Parse("2006-01-02", q.Start); err == nil {
			f.From = &t
		}
	}
	if q.End != "" {
		if t, err := time.Parse("2006-01-02", q.End); err == nil {
			next := t.AddDate(0, 0, 1)
			f.To = &next
		}
	}

	list, total, err := repository.SearchConversationRecords(tenantID, f)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ConversationRecord, 0, len(list))
	for _, c := range list {
		rec := ConversationRecord{
			ID: c.ID, CustomerID: c.CustomerID, ChannelID: c.ChannelID,
			AgentID: c.AgentID, Status: c.Status, Archived: c.Archived,
			MessageCount: c.MessageCount, LastMessage: c.LastMessage,
			LastMessageAt: fmtTime(c.LastMessageAt), CreatedAt: fmtTime(c.CreatedAt),
			FirstMessageAt: "",
		}
		if cu, err := repository.FindCustomerByID(c.CustomerID); err == nil {
			rec.CustomerName = cu.Nickname
			rec.CustomerAvatar = cu.Avatar
		}
		if ch, err := repository.FindChannelByID(c.TenantID, c.ChannelID); err == nil {
			rec.ChannelName = ch.Name
			rec.ChannelType = ch.Type
		}
		rec.AgentName = "未分配"
		if c.AgentID > 0 {
			if u, err := repository.FindUserBriefByID(c.AgentID); err == nil {
				rec.AgentName = u.Name
			}
		}
		// 首条消息（列表里预览对话开头）
		if first, err := repository.FindFirstUserMessage(c.ID); err == nil {
			rec.FirstMessage = messagePreview(first)
			rec.FirstMessageAt = fmtTime(first.CreatedAt)
		}
		out = append(out, rec)
	}
	return out, total, nil
}
