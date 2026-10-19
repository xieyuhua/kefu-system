package service

// ---------------- 客户域（Customer） ----------------
// customers、customer_tags（标签）、orders、products，
// 以及客户列表 / 详情 / 历史档案与订单商品查询。
//
// 形态约定：业务编排都是 CustomerService 的方法（指针接收者），构造时建好客户 / 订单 /
// 商品 / 标签 / 跨表只读五个仓储；跨表只读都在仓储内完成，不引入反向依赖。
import (
	"errors"

	"kefu-server/internal/common/dto"
	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/response"
	"kefu-server/pkg/storage"
	"kefu-server/pkg/util"
)

// CustomerService 客户内核的业务编排层。
// 客户、订单、商品、标签与跨表只读分属五个仓储：本结构体同时持有它们，
// 使「客户详情 / 客户列表」这类一次要拼多张表的编排不必跨 service 调用。
type CustomerService struct {
	repo     *repository.CustomerRepository
	orders   *repository.OrderRepository
	products *repository.ProductRepository
	tags     *repository.TagRepository
	refs     *repository.RefsRepository
}

// NewCustomerService 创建业务层实例（自带数据访问层依赖）。
func NewCustomerService() *CustomerService {
	return &CustomerService{
		repo:     repository.NewCustomerRepository(),
		orders:   repository.NewOrderRepository(),
		products: repository.NewProductRepository(),
		tags:     repository.NewTagRepository(),
		refs:     repository.NewRefsRepository(),
	}
}

// ---------------- Customer / 历史档案 ----------------

func (s *CustomerService) ListCustomers(tenantID uint, keyword string, channelID uint, page, pageSize int) (*response.Paged, error) {
	// 统一夹取到合法区间，并回显夹取后的值（否则 page_size=100000000 会原样出现在响应里）
	page, pageSize = storage.Default().ClampPage(page, pageSize)
	list, total, err := s.repo.ListCustomersPaged(tenantID, keyword, channelID, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		// 统计失败不阻断列表展示，与原逻辑一致
		oc, _ := s.orders.CountOrdersOfCustomer(c.ID)
		cc, _ := s.repo.CountConversationsOfCustomer(c.ID)
		item := dto.CustomerListItem{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: s.GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: s.CityOrLookup(c),
			OrderCount: oc, ConversationCount: cc,
			LastSeenAt: util.Default().FmtTimePtr(c.LastSeenAt), CreatedAt: util.Default().FmtTime(c.CreatedAt),
		}
		if ch, err := s.refs.FindChannelRefByID(tenantID, c.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
		}
		vos = append(vos, item)
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *CustomerService) GetCustomerDetail(tenantID, id uint) (*dto.CustomerDetail, error) {
	c, err := s.repo.FindCustomerInTenant(tenantID, id)
	if err != nil {
		return nil, errors.New("客户不存在")
	}
	detail := dto.CustomerDetail{
		User: dto.CustomerDetailUser{
			ID: c.ID, ExternalID: c.ExternalID, Nickname: c.Nickname,
			Avatar: c.Avatar, Phone: c.Phone, Remark: c.Remark,
			Tags: s.GetCustomerTags(tenantID, c.ID),
			IP:   c.IP, City: s.CityOrLookup(c),
			CreatedAt: util.Default().FmtTime(c.CreatedAt),
		},
		Orders:        []dto.OrderVO{},
		Conversations: []dto.HistoryConversationVO{},
	}
	// 统计失败按 0 处理，与原逻辑一致
	detail.User.OrderCount, _ = s.orders.CountOrdersOfCustomer(c.ID)

	if ch, err := s.refs.FindChannelRefByID(tenantID, c.ChannelID); err == nil {
		detail.User.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}

	orders, _ := s.orders.ListOrdersOfCustomer(c.ID, 50)
	for _, o := range orders {
		detail.Orders = append(detail.Orders, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: util.Default().FmtTime(o.CreatedAt), PaidAt: util.Default().FmtTimePtr(o.PaidAt),
		})
	}

	convs, _ := s.repo.ListCustomerConversations(tenantID, c.ID, 50)
	for _, cv := range convs {
		item := dto.HistoryConversationVO{
			ID: cv.ID, Status: cv.Status, MessageCount: cv.MessageCount,
			Summary: cv.Summary, CreatedAt: util.Default().FmtTime(cv.CreatedAt),
			ClosedAt: util.Default().FmtTimePtr(cv.ClosedAt), LastMessageAt: util.Default().FmtTime(cv.LastMessageAt),
		}
		if cn, err := s.refs.FindChannelRefByID(tenantID, cv.ChannelID); err == nil {
			item.Channel = &dto.ChannelRef{ID: cn.ID, Type: cn.Type, Name: cn.Name}
		}
		if cv.AgentID > 0 {
			if u, err := s.refs.FindUserRefByID(cv.AgentID); err == nil {
				item.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
			}
		}
		detail.Conversations = append(detail.Conversations, item)
	}
	return &detail, nil
}

// UpdateCustomerRemark 客服备注（同样跨客服可见）
func (s *CustomerService) UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return s.repo.SaveCustomerRemark(tenantID, customerID, remark)
}

// ---------------- 用户端兜底租户 ----------------

// DefaultTenantID 未登录、未显式传 ?tenant= 时取第一个租户（便于演示）；取不到返回 0。
// 调用方有两个且不能互相 import：client 端（用户端页面与访客 WS 的兜底租户）
// 与上传服务（游客态上传也要落 tenant_id），因此口径下沉到客户域。
func (s *CustomerService) DefaultTenantID() uint {
	id, err := s.refs.FirstTenantID()
	if err != nil {
		return 0
	}
	return id
}

// ---------------- 供其它域 / 端复用的读写门面 ----------------
// 同包内的会话域与 client / agent 两端需要「按主键取客户 / 订单 / 商品」
// 这类单表读取，以及「刷新客户最近活跃时间」这一处跨域写入。跨包调用只能走 Service：
// 这里的每个方法都只是把调用转给本包仓储，SQL 与原来逐字一致，不含额外业务规则。

// FindCustomerByID 按主键取客户（跨包只读门面）。
func (s *CustomerService) FindCustomerByID(customerID uint) (*model.Customer, error) {
	return s.repo.FindCustomerByID(customerID)
}

// FindCustomerByChannelExternalID 按「租户 + 渠道 + 外部标识」取客户（跨包只读门面）。
func (s *CustomerService) FindCustomerByChannelExternalID(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	return s.repo.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
}

// FindCustomerByExternalID 按「租户 + 外部标识」取客户（跨包只读门面）。
func (s *CustomerService) FindCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	return s.repo.FindCustomerByExternalID(tenantID, externalID)
}

// FindActiveConversationOfCustomer 取客户最近一个未结束且未归档的会话（跨包只读门面）。
func (s *CustomerService) FindActiveConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return s.repo.FindActiveConversationOfCustomer(tenantID, customerID)
}

// CreateCustomer 新增客户（跨包写入门面，供访客首次进入 / 演示入口建人）。
func (s *CustomerService) CreateCustomer(c *model.Customer) error {
	return s.repo.CreateCustomer(c)
}

// UpdateCustomerColumns 按主键更新客户的部分字段（跨包写入门面：刷新最近活跃时间、演示入口改昵称）。
func (s *CustomerService) UpdateCustomerColumns(id uint, updates map[string]interface{}) error {
	return s.repo.UpdateCustomerColumns(id, updates)
}
