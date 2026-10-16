package service

import (
	"errors"
	"log"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
)

// ---------------- 会话归档 / 删除 ----------------

// ArchiveConversation 归档会话（客服主动清理工作台列表）。
// 仅隐藏，不删除数据；客户再次发消息会自动恢复（见 UnarchiveOnNewMessage）。
func ArchiveConversation(tenantID, convID, operatorID uint) error {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
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
	return gormDB().Model(&model.Conversation{}).Where("id = ?", c.ID).Updates(updates).Error
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
	var rows []model.Conversation
	if err := gormDB().Select("id", "agent_id").
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&rows).Error; err != nil {
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
		if err := gormDB().Model(&model.Conversation{}).
			Where("id = ?", c.ID).Updates(updates).Error; err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// UnarchiveConversation 取消归档，恢复到工作台列表
func UnarchiveConversation(tenantID, convID uint) error {
	return gormDB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, convID).
		Updates(map[string]interface{}{"archived": false, "archived_at": nil}).Error
}

// DeleteConversation 彻底删除会话及其消息（不可恢复）
func DeleteConversation(tenantID, convID uint) error {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
		return errors.New("会话不存在")
	}
	gormDB().Where("tenant_id = ? AND conversation_id = ?", tenantID, c.ID).Delete(&model.Message{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, c.ID).Delete(&model.Conversation{}).Error
}

// ReopenConversation 客户重新发起咨询时调用：
// 若会话被归档，则取消归档并回到「未接待」状态（消息不丢）。
func ReopenConversation(tenantID, convID uint) {
	gormDB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ? AND archived = ?", tenantID, convID, true).
		Updates(map[string]interface{}{"archived": false, "archived_at": nil})
}

// ---------------- 客户在线状态（活跃度推断） ----------------

// onlineWindow 活跃判定窗口：超过此时长无活动视为离线（微信式）
const onlineWindow = 2 * time.Minute

// IsCustomerOnline 按最近活跃时间推断客户是否在线
func IsCustomerOnline(c model.Customer) bool {
	if c.LastSeenAt == nil {
		return false
	}
	return time.Since(*c.LastSeenAt) <= onlineWindow
}

// ---------------- 消息搜索 ----------------

// SearchMessages 在指定会话内按内容关键词搜索消息，返回升序结果
func SearchMessages(tenantID, convID uint, keyword string, limit int) ([]model.Message, error) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, errors.New("请输入搜索关键词")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []model.Message
	err := gormDB().
		Where("tenant_id = ? AND conversation_id = ? AND content LIKE ?", tenantID, convID, "%"+kw+"%").
		Order("id desc").Limit(limit).Find(&list).Error
	if err != nil {
		return nil, err
	}
	// 反转为升序，与消息流顺序一致
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, nil
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
	dbq := gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)

	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + kw + "%"
		// 昵称/手机号/外部ID 匹配，或该会话下存在内容匹配的消息
		dbq = dbq.Where(
			"(customer_id IN (SELECT id FROM customers WHERE nickname LIKE ? OR phone LIKE ? OR external_id LIKE ?) "+
				"OR id IN (SELECT conversation_id FROM messages WHERE content LIKE ?))",
			like, like, like, like,
		)
	}
	if q.ChannelID > 0 {
		dbq = dbq.Where("channel_id = ?", q.ChannelID)
	}
	if q.Status != "" && q.Status != "all" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	if q.AgentID > 0 {
		dbq = dbq.Where("agent_id = ?", q.AgentID)
	}
	if q.Start != "" {
		if t, err := time.Parse("2006-01-02", q.Start); err == nil {
			dbq = dbq.Where("created_at >= ?", t)
		}
	}
	if q.End != "" {
		if t, err := time.Parse("2006-01-02", q.End); err == nil {
			dbq = dbq.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}

	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []model.Conversation
	if err := dbq.Order("last_message_at desc").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).
		Find(&list).Error; err != nil {
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
		var cu model.Customer
		if gormDB().First(&cu, c.CustomerID).Error == nil {
			rec.CustomerName = cu.Nickname
			rec.CustomerAvatar = cu.Avatar
		}
		var ch model.Channel
		if gormDB().First(&ch, c.ChannelID).Error == nil {
			rec.ChannelName = ch.Name
			rec.ChannelType = ch.Type
		}
		rec.AgentName = "未分配"
		if c.AgentID > 0 {
			var u model.User
			if gormDB().First(&u, c.AgentID).Error == nil {
				rec.AgentName = u.Name
			}
		}
		// 首条消息（列表里预览对话开头）
		var first model.Message
		if gormDB().Where("conversation_id = ? AND type != 'system'", c.ID).
			Order("id asc").First(&first).Error == nil {
			rec.FirstMessage = messagePreview(first)
			rec.FirstMessageAt = fmtTime(first.CreatedAt)
		}
		out = append(out, rec)
	}
	return out, total, nil
}
