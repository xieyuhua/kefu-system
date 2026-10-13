package service

import (
	"errors"
	"strings"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
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
	q := gormDB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID)
	if channelID > 0 {
		q = q.Where("channel_id = ?", channelID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("nickname LIKE ? OR phone LIKE ? OR external_id LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Customer
	if err := q.Order("last_seen_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		var oc, cc int64
		gormDB().Model(&model.Order{}).Where("customer_id = ?", c.ID).Count(&oc)
		gormDB().Model(&model.Conversation{}).Where("customer_id = ?", c.ID).Count(&cc)
		item := dto.CustomerListItem{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags:       GetCustomerTags(tenantID, c.ID),
			OrderCount: oc, ConversationCount: cc,
			LastSeenAt: fmtTimePtr(c.LastSeenAt), CreatedAt: fmtTime(c.CreatedAt),
		}
		var ch model.Channel
		if gormDB().First(&ch, c.ChannelID).Error == nil {
			item.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
		}
		vos = append(vos, item)
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

func GetCustomerDetail(tenantID, id uint) (*dto.CustomerDetail, error) {
	var c model.Customer
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("客户不存在")
	}
	detail := dto.CustomerDetail{
		User: dto.CustomerDetailUser{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags:      GetCustomerTags(tenantID, c.ID),
			CreatedAt: fmtTime(c.CreatedAt),
		},
		Orders:        []dto.OrderVO{},
		Conversations: []dto.HistoryConversationVO{},
	}
	gormDB().Model(&model.Order{}).Where("customer_id = ?", c.ID).Count(&detail.User.OrderCount)

	var ch model.Channel
	if gormDB().First(&ch, c.ChannelID).Error == nil {
		detail.User.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}

	var orders []model.Order
	gormDB().Where("customer_id = ?", c.ID).Order("created_at desc").Limit(50).Find(&orders)
	for _, o := range orders {
		detail.Orders = append(detail.Orders, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: fmtTime(o.CreatedAt), PaidAt: fmtTimePtr(o.PaidAt),
		})
	}

	var convs []model.Conversation
	gormDB().Where("tenant_id = ? AND customer_id = ?", tenantID, c.ID).
		Order("created_at desc").Limit(50).Find(&convs)
	for _, cv := range convs {
		item := dto.HistoryConversationVO{
			ID: cv.ID, Status: cv.Status, MessageCount: cv.MessageCount,
			Summary: cv.Summary, CreatedAt: fmtTime(cv.CreatedAt),
			ClosedAt: fmtTimePtr(cv.ClosedAt), LastMessageAt: fmtTime(cv.LastMessageAt),
		}
		var cn model.Channel
		if gormDB().First(&cn, cv.ChannelID).Error == nil {
			item.Channel = &dto.ChannelRef{ID: cn.ID, Type: cn.Type, Name: cn.Name}
		}
		if cv.AgentID > 0 {
			var u model.User
			if gormDB().First(&u, cv.AgentID).Error == nil {
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
	q := gormDB().Model(&model.Order{}).Where("tenant_id = ?", tenantID)
	if customerID > 0 {
		q = q.Where("customer_id = ?", customerID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("order_no LIKE ? OR title LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Order
	if err := q.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
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
	q := gormDB().Model(&model.Product{}).Where("tenant_id = ?", tenantID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR p_id LIKE ? OR sku LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Product
	if err := q.Order("id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
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
	if err := gormDB().Create(&d).Error; err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: fmtTime(d.CreatedAt),
	}, nil
}

func ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	var list []model.Device
	if err := gormDB().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
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
	return gormDB().Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		Delete(&model.Device{}).Error
}

// ---------------- 离线消息 / 离线提醒 ----------------

// OfflineMessages 供 APP / PC 桌面端断线重连后增量同步。sinceID 为端侧已收到的最大消息 ID。
func OfflineMessages(tenantID, userID uint, sinceID uint, limit int) ([]dto.MessageVO, uint, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	// 只同步该客服可见范围内的会话消息：自己接待的 + 自己渠道下未分配的
	var chIDs []uint
	gormDB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)

	var convIDs []uint
	qc := gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if len(chIDs) > 0 {
		qc = qc.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
	} else {
		qc = qc.Where("agent_id = ?", userID)
	}
	qc.Pluck("id", &convIDs)
	if len(convIDs) == 0 {
		return []dto.MessageVO{}, sinceID, false, nil
	}
	var list []model.Message
	q := gormDB().Where("tenant_id = ? AND conversation_id IN ? AND id > ?", tenantID, convIDs, sinceID)
	if err := q.Order("id asc").Limit(limit + 1).Find(&list).Error; err != nil {
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
	var list []model.Notification
	if err := gormDB().Where("tenant_id = ? AND user_id = ? AND id > ?", tenantID, userID, sinceID).
		Order("id asc").Limit(200).Find(&list).Error; err != nil {
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
	q := gormDB().Model(&model.Notification{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("read", true).Error
}
