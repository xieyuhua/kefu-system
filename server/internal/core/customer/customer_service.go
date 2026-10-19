// Package customer 客户内核：customers、customer_tags（标签）、orders、products，
// 以及客户列表 / 详情 / 历史档案与订单商品查询。模块内按 service / repository 分层，
// 少量跨端 HTTP 处理器见 handler.go。
//
// 依赖方向：core/customer → platform → pkg（不依赖任何业务端模块，也不依赖其它内核包）。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），Service 在构造时建好自己的
// Repository 依赖；本包的跨表只读都在 Repository 内完成，因此不引入反向内核依赖。
package customer

import (
	"errors"

	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/ipinfo"
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/response"
	"kefu-server/pkg/storage"
	"kefu-server/pkg/util"
)

// Service 客户内核的业务编排层。
type Service struct {
	repo *Repository
}

// NewService 创建业务层实例（自带数据访问层依赖）。
func NewService() *Service { return &Service{repo: NewRepository()} }

// ---------------- Customer / 历史档案 ----------------

func (s *Service) ListCustomers(tenantID uint, keyword string, channelID uint, page, pageSize int) (*response.Paged, error) {
	// 统一夹取到合法区间，并回显夹取后的值（否则 page_size=100000000 会原样出现在响应里）
	page, pageSize = storage.ClampPage(page, pageSize)
	list, total, err := s.repo.ListCustomersPaged(tenantID, keyword, channelID, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		// 统计失败不阻断列表展示，与原逻辑一致
		oc, _ := s.repo.CountOrdersOfCustomer(c.ID)
		cc, _ := s.repo.CountConversationsOfCustomer(c.ID)
		item := dto.CustomerListItem{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: s.GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: ipinfo.CityOrLookup(c),
			OrderCount: oc, ConversationCount: cc,
			LastSeenAt: util.FmtTimePtr(c.LastSeenAt), CreatedAt: util.FmtTime(c.CreatedAt),
		}
		if ch, err := s.repo.findChannelByID(tenantID, c.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
		}
		vos = append(vos, item)
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Service) GetCustomerDetail(tenantID, id uint) (*dto.CustomerDetail, error) {
	c, err := s.repo.FindCustomerInTenant(tenantID, id)
	if err != nil {
		return nil, errors.New("客户不存在")
	}
	detail := dto.CustomerDetail{
		User: dto.CustomerDetailUser{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: s.GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: ipinfo.CityOrLookup(c),
			CreatedAt: util.FmtTime(c.CreatedAt),
		},
		Orders:        []dto.OrderVO{},
		Conversations: []dto.HistoryConversationVO{},
	}
	// 统计失败按 0 处理，与原逻辑一致
	detail.User.OrderCount, _ = s.repo.CountOrdersOfCustomer(c.ID)

	if ch, err := s.repo.findChannelByID(tenantID, c.ChannelID); err == nil {
		detail.User.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}

	orders, _ := s.repo.ListOrdersOfCustomer(c.ID, 50)
	for _, o := range orders {
		detail.Orders = append(detail.Orders, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: util.FmtTime(o.CreatedAt), PaidAt: util.FmtTimePtr(o.PaidAt),
		})
	}

	convs, _ := s.repo.listConversationsByCustomer(tenantID, c.ID, 50)
	for _, cv := range convs {
		item := dto.HistoryConversationVO{
			ID: cv.ID, Status: cv.Status, MessageCount: cv.MessageCount,
			Summary: cv.Summary, CreatedAt: util.FmtTime(cv.CreatedAt),
			ClosedAt: util.FmtTimePtr(cv.ClosedAt), LastMessageAt: util.FmtTime(cv.LastMessageAt),
		}
		if cn, err := s.repo.findChannelByID(tenantID, cv.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: cn.ID, Type: cn.Type, Name: cn.Name}
		}
		if cv.AgentID > 0 {
			if u, err := s.repo.findUserBriefByID(cv.AgentID); err == nil {
				item.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
			}
		}
		detail.Conversations = append(detail.Conversations, item)
	}
	return &detail, nil
}

// UpdateCustomerRemark 客服备注（同样跨客服可见）
func (s *Service) UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return s.repo.SaveCustomerRemark(tenantID, customerID, remark)
}

// ---------------- 用户端兜底租户 ----------------

// DefaultTenantID 未登录、未显式传 ?tenant= 时取第一个租户（便于演示）；取不到返回 0。
// 调用方有两个且不能互相 import：modules/client（用户端页面与访客 WS 的兜底租户）
// 与 core/upload（游客态上传也要落 tenant_id），因此口径下沉到客户内核。
func (s *Service) DefaultTenantID() uint {
	id, err := s.repo.FirstTenantID()
	if err != nil {
		return 0
	}
	return id
}

// ---------------- 供其它内核 / 端复用的读写门面 ----------------
// core/conversation、core/upload 与 modules/client、modules/agent 需要「按主键取客户 / 订单 / 商品」
// 这类单表读取，以及「刷新客户最近活跃时间」这一处跨内核写入。跨包只能走 Service：
// 这里的每个方法都只是把调用转给本包 repository，SQL 与原来逐字一致，不含额外业务规则。

// FindCustomerByID 按主键取客户（跨包只读门面）。
func (s *Service) FindCustomerByID(customerID uint) (*model.Customer, error) {
	return s.repo.FindCustomerByID(customerID)
}

// FindCustomerByChannelExternalID 按「租户 + 渠道 + 外部标识」取客户（跨包只读门面）。
func (s *Service) FindCustomerByChannelExternalID(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	return s.repo.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
}

// FindCustomerByExternalID 按「租户 + 外部标识」取客户（跨包只读门面）。
func (s *Service) FindCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	return s.repo.FindCustomerByExternalID(tenantID, externalID)
}

// FindActiveConversationOfCustomer 取客户最近一个未结束且未归档的会话（跨包只读门面）。
func (s *Service) FindActiveConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return s.repo.FindActiveConversationOfCustomer(tenantID, customerID)
}

// CreateCustomer 新增客户（跨包写入门面，供访客首次进入 / 演示入口建人）。
func (s *Service) CreateCustomer(c *model.Customer) error {
	return s.repo.CreateCustomer(c)
}

// UpdateCustomerColumns 按主键更新客户的部分字段（跨包写入门面：刷新最近活跃时间、演示入口改昵称）。
func (s *Service) UpdateCustomerColumns(id uint, updates map[string]interface{}) error {
	return s.repo.UpdateCustomerColumns(id, updates)
}
