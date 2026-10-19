package service

import (
	"errors"

	"kefu-server/internal/common/model"
	"kefu-server/internal/platform/dto"
)

// ---------------- 渠道 ↔ 接待账号快速设置 ----------------

// ListChannelAgents 列出某渠道可分配的客服账号及当前是否已分配
func (s *ChannelService) ListChannelAgents(tenantID, channelID uint) ([]dto.ChannelAgentVO, error) {
	if _, err := s.repo.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	users, err := s.repo.ListTenantAgents(tenantID)
	if err != nil {
		return nil, err
	}
	selected := map[uint]bool{}
	acs, _ := s.repo.ListAgentChannelsByChannel(channelID)
	for _, ac := range acs {
		selected[ac.UserID] = true
	}
	out := make([]dto.ChannelAgentVO, 0, len(users))
	for _, u := range users {
		out = append(out, dto.ChannelAgentVO{
			ID: u.ID, Name: u.Name, Account: u.Account, Avatar: u.Avatar,
			Online: u.Online, Enabled: u.Enabled, Selected: selected[u.ID],
		})
	}
	return out, nil
}

// SetChannelAgents 批量设置某渠道的接待账号（全量覆盖）
func (s *ChannelService) SetChannelAgents(tenantID, channelID uint, userIDs []uint) ([]dto.ChannelAgentVO, error) {
	if _, err := s.repo.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	// 校验账号归属本租户
	valid := map[uint]bool{}
	if len(userIDs) > 0 {
		// 校验失败按「无账号通过校验」处理，与原逻辑一致
		users, _ := s.repo.FindAgentUsersByIDs(tenantID, userIDs)
		for _, u := range users {
			valid[u.ID] = true
		}
	}
	// 覆盖式写入：清空旧分配后逐个重建（写入失败由后续列表查询体现，与原逻辑一致）
	_ = s.repo.DeleteAgentChannels(channelID)
	for _, id := range userIDs {
		if !valid[id] {
			continue
		}
		_ = s.repo.CreateAgentChannel(&model.AgentChannel{UserID: id, ChannelID: channelID})
	}
	return s.ListChannelAgents(tenantID, channelID)
}

// HasAgentChannel 判断客服是否配置了接待该渠道（跨包只读门面）。
func (s *ChannelService) HasAgentChannel(userID, channelID uint) (bool, error) {
	return s.repo.HasAgentChannel(userID, channelID)
}

// ListAgentChannelsByChannel 列出某渠道已分配的账号关联（跨包只读门面）。
func (s *ChannelService) ListAgentChannelsByChannel(channelID uint) ([]*model.AgentChannel, error) {
	return s.repo.ListAgentChannelsByChannel(channelID)
}
