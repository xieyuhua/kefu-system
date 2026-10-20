package service

// 本文件是 common/service 的业务层：附件上传不涉及自有数据表（无 repository），
// 依赖有两块：一是「游客态兜底租户」这一口径（跨包只能经客户内核的 Service 取得），
// 二是对象存储配置（settings 表里的 storage.object，见 object_storage_service.go）。
//
// 形态约定：对外能力是 Service 的方法（指针接收者），跨内核依赖在构造时建好。

import (
	"kefu-server/pkg/oss"
)

// UploadService 附件上传内核的业务层。
type UploadService struct {
	customer *CustomerService
	objects  *ObjectStorageService
}

// NewUploadService 创建业务层实例（自带客户内核与对象存储配置依赖）。
func NewUploadService() *UploadService {
	return &UploadService{customer: NewCustomerService(), objects: NewObjectStorageService()}
}

// DefaultTenantID 游客态（未登录、未传 ?tenant=）的兜底租户，与用户端同口径。
// 兜底口径本身下沉在 common/service，这里只做本模块的入口。
func (s *UploadService) DefaultTenantID() uint {
	return s.customer.DefaultTenantID()
}

// ObjectStorageClient 返回租户已启用的对象存储客户端。
//   - 未启用 / 配置不完整：(nil, nil, nil)，调用方保持原有本地磁盘存储；
//   - 配置有误：返回错误，由调用方明确报错（不静默降级到本地）。
func (s *UploadService) ObjectStorageClient(tenantID uint) (*oss.Client, *oss.Config, error) {
	return s.objects.ObjectClient(tenantID)
}
