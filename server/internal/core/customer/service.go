// Package customer 客户内核：customers、customer_tags（标签）、orders、products，
// 以及客户列表 / 详情 / 历史档案与订单商品查询。模块内按 service / repository 分层，
// 少量跨端 HTTP 处理器见 handler.go。
//
// 依赖方向：core/customer → platform → pkg（不依赖任何业务端模块，也不依赖其它内核包）。
package customer

import (
	"errors"
	"strings"
	"time"

	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/ipinfo"
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/response"
	"kefu-server/pkg/util"
)

// ---------------- Customer / 历史档案 ----------------

func ListCustomers(tenantID uint, keyword string, channelID uint, page, pageSize int) (*response.Paged, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	list, total, err := ListCustomersPaged(tenantID, keyword, channelID, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		// 统计失败不阻断列表展示，与原逻辑一致
		oc, _ := CountOrdersOfCustomer(c.ID)
		cc, _ := CountConversationsOfCustomer(c.ID)
		item := dto.CustomerListItem{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: ipinfo.CityOrLookup(c),
			OrderCount: oc, ConversationCount: cc,
			LastSeenAt: util.FmtTimePtr(c.LastSeenAt), CreatedAt: util.FmtTime(c.CreatedAt),
		}
		if ch, err := findChannelByID(tenantID, c.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
		}
		vos = append(vos, item)
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

func GetCustomerDetail(tenantID, id uint) (*dto.CustomerDetail, error) {
	c, err := FindCustomerInTenant(tenantID, id)
	if err != nil {
		return nil, errors.New("客户不存在")
	}
	detail := dto.CustomerDetail{
		User: dto.CustomerDetailUser{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: ipinfo.CityOrLookup(c),
			CreatedAt: util.FmtTime(c.CreatedAt),
		},
		Orders:        []dto.OrderVO{},
		Conversations: []dto.HistoryConversationVO{},
	}
	// 统计失败按 0 处理，与原逻辑一致
	detail.User.OrderCount, _ = CountOrdersOfCustomer(c.ID)

	if ch, err := findChannelByID(tenantID, c.ChannelID); err == nil {
		detail.User.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}

	orders, _ := ListOrdersOfCustomer(c.ID, 50)
	for _, o := range orders {
		detail.Orders = append(detail.Orders, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: util.FmtTime(o.CreatedAt), PaidAt: util.FmtTimePtr(o.PaidAt),
		})
	}

	convs, _ := listConversationsByCustomer(tenantID, c.ID, 50)
	for _, cv := range convs {
		item := dto.HistoryConversationVO{
			ID: cv.ID, Status: cv.Status, MessageCount: cv.MessageCount,
			Summary: cv.Summary, CreatedAt: util.FmtTime(cv.CreatedAt),
			ClosedAt: util.FmtTimePtr(cv.ClosedAt), LastMessageAt: util.FmtTime(cv.LastMessageAt),
		}
		if cn, err := findChannelByID(tenantID, cv.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: cn.ID, Type: cn.Type, Name: cn.Name}
		}
		if cv.AgentID > 0 {
			if u, err := findUserBriefByID(cv.AgentID); err == nil {
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
	list, total, err := ListOrdersPaged(tenantID, customerID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, o := range list {
		vos = append(vos, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: util.FmtTime(o.CreatedAt), PaidAt: util.FmtTimePtr(o.PaidAt),
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
	list, total, err := ListProductsPaged(tenantID, keyword, page, pageSize)
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

// ---------------- 客户标签 ----------------
// 客服给客户打标签 → 其他客服接待同一客户时能看到（会话列表、客户信息栏、历史档案均展示）

type TagVO struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Sort      int    `json:"sort"`
	UseCount  int64  `json:"use_count"`
	CreatedAt string `json:"created_at"`
}

type TagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  int    `json:"sort"`
}

func ListTags(tenantID uint) ([]TagVO, error) {
	list, err := ListTagsByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]TagVO, 0, len(list))
	for _, t := range list {
		// 使用次数统计失败按 0 处理，不阻断列表展示
		uc, _ := CountTagUsage(tenantID, t.ID)
		out = append(out, TagVO{
			ID: t.ID, Name: t.Name, Color: t.Color, Sort: t.Sort,
			UseCount: uc, CreatedAt: util.FmtTime(t.CreatedAt),
		})
	}
	return out, nil
}

func CreateTag(tenantID uint, req TagRequest) (*TagVO, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("标签名称不能为空")
	}
	if _, err := FindTagByName(tenantID, name); err == nil {
		return nil, errors.New("标签已存在")
	}
	color := req.Color
	if color == "" {
		color = "blue"
	}
	t := &model.Tag{TenantID: tenantID, Name: name, Color: color, Sort: req.Sort}
	if err := InsertTag(t); err != nil {
		return nil, err
	}
	return &TagVO{ID: t.ID, Name: t.Name, Color: t.Color, Sort: t.Sort, CreatedAt: util.FmtTime(t.CreatedAt)}, nil
}

func DeleteTag(tenantID, id uint) error {
	return DeleteTagWithUsages(tenantID, id)
}

// GetCustomerTags 读取某客户的标签
func GetCustomerTags(tenantID, customerID uint) []dto.TagRef {
	rels, err := ListCustomerTags(tenantID, customerID)
	if err != nil {
		return []dto.TagRef{}
	}
	if len(rels) == 0 {
		return []dto.TagRef{}
	}
	ids := make([]uint, 0, len(rels))
	for _, r := range rels {
		ids = append(ids, r.TagID)
	}
	tags, _ := ListTagsByIDs(tenantID, ids)
	out := make([]dto.TagRef, 0, len(tags))
	for _, t := range tags {
		out = append(out, dto.TagRef{ID: t.ID, Name: t.Name, Color: t.Color})
	}
	return out
}

// SetCustomerTags 全量覆盖客户的标签集合
func SetCustomerTags(tenantID, customerID, operatorID uint, tagIDs []uint) ([]dto.TagRef, error) {
	if _, err := FindCustomerInTenant(tenantID, customerID); err != nil {
		return nil, errors.New("客户不存在")
	}
	// 只接受本租户真实存在的标签，非法 ID 一律忽略（与原逻辑一致）
	valid := map[uint]bool{}
	if len(tagIDs) > 0 {
		tags, _ := ListTagsByIDs(tenantID, tagIDs)
		for _, t := range tags {
			valid[t.ID] = true
		}
	}
	// 先删掉不再需要的旧关联
	old, _ := ListCustomerTags(tenantID, customerID)
	for _, o := range old {
		if !valid[o.TagID] {
			_ = DeleteCustomerTag(tenantID, customerID, o.TagID)
		}
	}
	// 再补齐新增的关联（已存在则跳过）
	now := time.Now()
	for _, id := range tagIDs {
		if !valid[id] {
			continue
		}
		cnt, _ := CountCustomerTag(tenantID, customerID, id)
		if cnt == 0 {
			_ = CreateCustomerTag(&model.CustomerTag{
				TenantID: tenantID, CustomerID: customerID, TagID: id,
				CreatedBy: operatorID, CreatedAt: now,
			})
		}
	}
	return GetCustomerTags(tenantID, customerID), nil
}

// UpdateCustomerRemark 客服备注（同样跨客服可见）
func UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return SaveCustomerRemark(tenantID, customerID, remark)
}

// ---------------- 用户端兜底租户 ----------------

// DefaultTenantID 未登录、未显式传 ?tenant= 时取第一个租户（便于演示）；取不到返回 0。
// 调用方有两个且不能互相 import：modules/client（用户端页面与访客 WS 的兜底租户）
// 与 core/upload（游客态上传也要落 tenant_id），因此口径下沉到客户内核。
func DefaultTenantID() uint {
	id, err := FirstTenantID()
	if err != nil {
		return 0
	}
	return id
}
