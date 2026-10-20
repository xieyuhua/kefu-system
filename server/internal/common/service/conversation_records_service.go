package service

import (
	"time"

	"kefu-server/internal/common/repository"
	"kefu-server/pkg/util"
)

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
	// UnrepliedTS 最后一条「客户消息」的时间（Unix 毫秒）；客服已回复 / 无客户消息为 0。
	// 与工作台会话列表（dto.ConversationVO.UnrepliedTS）同一口径、同一批量查询。
	UnrepliedTS    int64  `json:"unreplied_ts"`
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
func (s *ConversationService) SearchConversations(tenantID uint, q ConversationRecordsQuery) ([]ConversationRecord, int64, error) {
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

	list, total, err := s.repo.SearchConversationRecords(tenantID, f)
	if err != nil {
		return nil, 0, err
	}

	// 「未回复」口径与工作台列表完全一致：整页批量取每个会话的最后一条消息（1 条聚合 SQL），
	// 再按「最后一条是不是客户发的」填充 UnrepliedTS；取不到时保持 0（不影响记录本身）。
	ids := make([]uint, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	lasts, err := s.messages.FindLastMessagesOfConversations(tenantID, ids)
	if err != nil {
		lasts = nil
	}

	out := make([]ConversationRecord, 0, len(list))
	for _, c := range list {
		rec := ConversationRecord{
			ID: c.ID, CustomerID: c.CustomerID, ChannelID: c.ChannelID,
			AgentID: c.AgentID, Status: c.Status, Archived: c.Archived,
			MessageCount: c.MessageCount, LastMessage: c.LastMessage,
			LastMessageAt: util.Default().FmtTime(c.LastMessageAt), CreatedAt: util.Default().FmtTime(c.CreatedAt),
			UnrepliedTS:    unrepliedTS(lasts[c.ID]),
			FirstMessageAt: "",
		}
		if cu, err := s.customer.FindCustomerByID(c.CustomerID); err == nil {
			rec.CustomerName = cu.Nickname
			rec.CustomerAvatar = cu.Avatar
		}
		if ch, err := s.channel.FindChannelByID(c.TenantID, c.ChannelID); err == nil {
			rec.ChannelName = ch.Name
			rec.ChannelType = ch.Type
		}
		rec.AgentName = "未分配"
		if c.AgentID > 0 {
			if u, err := s.account.FindUserBriefByID(c.AgentID); err == nil {
				rec.AgentName = u.Name
			}
		}
		// 首条消息（列表里预览对话开头）
		if first, err := s.messages.FindFirstUserMessage(c.ID); err == nil {
			rec.FirstMessage = messagePreview(first)
			rec.FirstMessageAt = util.Default().FmtTime(first.CreatedAt)
		}
		out = append(out, rec)
	}
	return out, total, nil
}

func (s *ConversationService) agentName(id uint) string {
	if u, err := s.account.FindUserBriefByID(id); err == nil && u.Name != "" {
		return u.Name
	}
	return "客服"
}
