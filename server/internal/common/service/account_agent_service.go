package service

import (
	"errors"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/dto"
)

// orDefault 空串取默认值（渠道名/昵称兜底）
func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// ---------------- 子账号（Agent） ----------------

func (s *AccountService) ListAgents(tenantID uint) ([]dto.AgentVO, error) {
	users, err := s.repo.ListUsersByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AgentVO, 0, len(users))
	for _, u := range users {
		vo := dto.AgentVO{
			ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
			Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
			Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
		}
		vo.Status = "offline"
		if u.Online {
			vo.Status = "online"
		}
		// 该账号可接待的渠道；查询失败不阻断列表展示，保持空渠道列表（与原逻辑一致）
		if chs, err := s.repo.UserListChannelsByTenant(tenantID, u.ID, false); err == nil {
			for _, ch := range chs {
				vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
			}
		}
		// 进行中会话数；统计失败按 0 处理（与原逻辑一致）
		if active, err := s.repo.CountOpenConversations(tenantID, u.ID); err == nil {
			vo.ActiveCount = active
		}
		out = append(out, vo)
	}
	return out, nil
}

func (s *AccountService) CreateAgent(tenantID uint, req dto.AgentRequest) (*dto.AgentVO, error) {
	if req.Account == "" || req.Password == "" {
		return nil, errors.New("账号与密码不能为空")
	}
	// 账号全局唯一；统计失败按「未被占用」处理（与原逻辑一致）
	if cnt, _ := s.repo.CountUsersByAccount(req.Account, 0); cnt > 0 {
		return nil, errors.New("账号已存在")
	}
	hash, err := hashPwd(req.Password)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role != "admin" {
		role = "agent"
	}
	maxC := req.MaxConcurrent
	if maxC <= 0 {
		maxC = 5
	}
	u := &model.User{
		TenantID:      tenantID,
		Account:       req.Account,
		PasswordHash:  hash,
		Name:          orDefault(req.Name, req.Account),
		Role:          role,
		Avatar:        "/demo/avatar-2.svg",
		MaxConcurrent: maxC,
		Enabled:       true,
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := s.repo.CreateUser(u); err != nil {
		return nil, err
	}
	s.setAgentChannels(tenantID, u.ID, req.ChannelIDs)
	return s.getAgentVO(tenantID, u.ID)
}

func (s *AccountService) UpdateAgent(tenantID, id uint, req dto.AgentRequest) error {
	u, err := s.repo.FindUserByID(tenantID, id)
	if err != nil {
		return errors.New("账号不存在")
	}
	if req.Name != "" {
		u.Name = req.Name
	}
	if req.Role != "" {
		u.Role = req.Role
	}
	if req.MaxConcurrent > 0 {
		u.MaxConcurrent = req.MaxConcurrent
	}
	if req.Password != "" {
		h, err := hashPwd(req.Password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := s.repo.SaveUser(u); err != nil {
		return err
	}
	if req.ChannelIDs != nil {
		s.setAgentChannels(tenantID, id, req.ChannelIDs)
	}
	return nil
}

func (s *AccountService) DeleteAgent(tenantID, id uint) error {
	// 账号删除与渠道分配清理放在同一事务里，避免残留孤儿关联
	return s.repo.DeleteUserWithBindings(tenantID, id)
}

func (s *AccountService) SetOnline(tenantID, id uint, online bool) error {
	return s.repo.UpdateUserOnline(tenantID, id, online)
}

func (s *AccountService) setAgentChannels(tenantID, userID uint, ids []uint) {
	// 先清空该账号的渠道分配；删除失败不阻断主流程（与原逻辑一致）
	_ = s.repo.DeleteAgentChannelsByUser(userID)
	// 只允许分配到本租户自己的渠道：isAdmin 传 true 表示取租户全部渠道
	valid, _ := s.repo.UserListChannelsByTenant(tenantID, 0, true)
	allowed := map[uint]bool{}
	for _, c := range valid {
		allowed[c.ID] = true
	}
	for _, id := range ids {
		if !allowed[id] {
			continue
		}
		_ = s.repo.UserCreateAgentChannel(&model.AgentChannel{UserID: userID, ChannelID: id})
	}
}

func (s *AccountService) getAgentVO(tenantID, id uint) (*dto.AgentVO, error) {
	u, err := s.repo.FindUserByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	vo := &dto.AgentVO{
		ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
		Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
		Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
	}
	vo.Status = "offline"
	if u.Online {
		vo.Status = "online"
	}
	// 该账号可接待的渠道；查询失败保持空列表（与原逻辑一致）
	if chs, err := s.repo.UserListChannelsByTenant(tenantID, id, false); err == nil {
		for _, ch := range chs {
			vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
		}
	}
	return vo, nil
}
