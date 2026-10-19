package service

import (
	"errors"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
	"kefu-server/internal/response"
)

// ---------------- Customer / 历史档案 ----------------

func ListCustomers(tenantID uint, keyword string, channelID uint, page, pageSize int) (*response.Paged, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	list, total, err := repository.ListCustomersPaged(tenantID, keyword, channelID, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		// 统计失败不阻断列表展示，与原逻辑一致
		oc, _ := repository.CountOrdersOfCustomer(c.ID)
		cc, _ := repository.CountConversationsOfCustomer(c.ID)
		item := dto.CustomerListItem{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: cityOrLookup(c),
			OrderCount: oc, ConversationCount: cc,
			LastSeenAt: fmtTimePtr(c.LastSeenAt), CreatedAt: fmtTime(c.CreatedAt),
		}
		if ch, err := repository.FindChannelByID(tenantID, c.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
		}
		vos = append(vos, item)
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

func GetCustomerDetail(tenantID, id uint) (*dto.CustomerDetail, error) {
	c, err := repository.FindCustomerInTenant(tenantID, id)
	if err != nil {
		return nil, errors.New("客户不存在")
	}
	detail := dto.CustomerDetail{
		User: dto.CustomerDetailUser{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: cityOrLookup(*c),
			CreatedAt: fmtTime(c.CreatedAt),
		},
		Orders:        []dto.OrderVO{},
		Conversations: []dto.HistoryConversationVO{},
	}
	// 统计失败按 0 处理，与原逻辑一致
	detail.User.OrderCount, _ = repository.CountOrdersOfCustomer(c.ID)

	if ch, err := repository.FindChannelByID(tenantID, c.ChannelID); err == nil {
		detail.User.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}

	orders, _ := repository.ListOrdersOfCustomer(c.ID, 50)
	for _, o := range orders {
		detail.Orders = append(detail.Orders, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: fmtTime(o.CreatedAt), PaidAt: fmtTimePtr(o.PaidAt),
		})
	}

	convs, _ := repository.ListConversationsByCustomer(tenantID, c.ID, 50)
	for _, cv := range convs {
		item := dto.HistoryConversationVO{
			ID: cv.ID, Status: cv.Status, MessageCount: cv.MessageCount,
			Summary: cv.Summary, CreatedAt: fmtTime(cv.CreatedAt),
			ClosedAt: fmtTimePtr(cv.ClosedAt), LastMessageAt: fmtTime(cv.LastMessageAt),
		}
		if cn, err := repository.FindChannelByID(tenantID, cv.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: cn.ID, Type: cn.Type, Name: cn.Name}
		}
		if cv.AgentID > 0 {
			if u, err := repository.FindUserBriefByID(cv.AgentID); err == nil {
				item.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
			}
		}
		detail.Conversations = append(detail.Conversations, item)
	}
	return &detail, nil
}

// ---------------- Order ----------------

func ListOrders(tenantID uint, customerID uint, keyword string, page, pageSize int) (*response.Paged, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	list, total, err := repository.ListOrdersPaged(tenantID, customerID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, o := range list {
		vos = append(vos, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: fmtTime(o.CreatedAt), PaidAt: fmtTimePtr(o.PaidAt),
		})
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

// ---------------- Product ----------------

func ListProducts(tenantID uint, keyword string, page, pageSize int) (*response.Paged, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	list, total, err := repository.ListProductsPaged(tenantID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, p := range list {
		vos = append(vos, dto.ProductVO{
			ID: p.PID, Title: p.Title, Price: p.Price, Image: p.Image,
			Stock: p.Stock, SKU: p.SKU, URL: p.URL,
		})
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

// ---------------- Device（APP / PC 桌面端） ----------------

func RegisterDevice(tenantID, userID uint, req dto.DeviceRequest) (*dto.DeviceVO, error) {
	if req.Device == "" {
		req.Device = "app"
	}
	d := model.Device{
		TenantID:  tenantID,
		UserID:    userID,
		Device:    req.Device,
		Platform:  req.Platform,
		PushToken: req.PushToken,
		Name:      req.Name,
	}
	if err := repository.CreateDevice(&d); err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: fmtTime(d.CreatedAt),
	}, nil
}

func ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	list, err := repository.ListDevices(tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DeviceVO, 0, len(list))
	for _, d := range list {
		out = append(out, dto.DeviceVO{
			ID: d.ID, Device: d.Device, Platform: d.Platform,
			Name: d.Name, PushToken: d.PushToken, CreatedAt: fmtTime(d.CreatedAt),
		})
	}
	return out, nil
}

func DeleteDevice(tenantID, userID, id uint) error {
	return repository.DeleteDevice(tenantID, userID, id)
}

// ---------------- 离线消息 / 离线提醒 ----------------

// OfflineMessages 供 APP / PC 桌面端断线重连后增量同步。sinceID 为端侧已收到的最大消息 ID。
func OfflineMessages(tenantID, userID uint, sinceID uint, limit int) ([]dto.MessageVO, uint, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	// 只同步该客服可见范围内的会话消息：自己接待的 + 自己渠道下未分配的
	convIDs, err := repository.ListVisibleConversationIDs(tenantID, userID)
	if err != nil {
		return nil, sinceID, false, err
	}
	if len(convIDs) == 0 {
		return []dto.MessageVO{}, sinceID, false, nil
	}
	// 多取一条用于判断是否还有更多
	list, err := repository.ListMessagesOfConversations(tenantID, convIDs, sinceID, limit+1)
	if err != nil {
		return nil, sinceID, false, err
	}
	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	out := make([]dto.MessageVO, 0, len(list))
	latest := sinceID
	for _, m := range list {
		out = append(out, buildMessageVO(m))
		if m.ID > latest {
			latest = m.ID
		}
	}
	return out, latest, hasMore, nil
}

func OfflineNotifications(tenantID, userID uint, sinceID uint) ([]dto.NotificationVO, uint, error) {
	list, err := repository.ListNotificationsAfter(tenantID, userID, sinceID, 200)
	if err != nil {
		return nil, sinceID, err
	}
	out := make([]dto.NotificationVO, 0, len(list))
	latest := sinceID
	for _, n := range list {
		out = append(out, dto.NotificationVO{
			ID: n.ID, Type: n.Type, Title: n.Title, Body: n.Body,
			ConversationID: n.ConversationID, Read: n.Read, CreatedAt: fmtTime(n.CreatedAt),
		})
		if n.ID > latest {
			latest = n.ID
		}
	}
	return out, latest, nil
}

func MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	return repository.MarkNotificationsRead(tenantID, userID, ids)
}
