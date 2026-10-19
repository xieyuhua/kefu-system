package service

// 本文件是 common/service 的业务层：附件上传不涉及自有数据表（无 repository），
// 唯一依赖是「游客态兜底租户」这一口径，跨包只能经客户内核的 Service 取得。
//
// 形态约定：对外能力是 Service 的方法（指针接收者），跨内核依赖在构造时建好。

// UploadService 附件上传内核的业务层。
type UploadService struct {
	customer *CustomerService
}

// NewUploadService 创建业务层实例（自带客户内核依赖）。
func NewUploadService() *UploadService { return &UploadService{customer: NewCustomerService()} }

// DefaultTenantID 游客态（未登录、未传 ?tenant=）的兜底租户，与用户端同口径。
// 兜底口径本身下沉在 common/service，这里只做本模块的入口。
func (s *UploadService) DefaultTenantID() uint {
	return s.customer.DefaultTenantID()
}
