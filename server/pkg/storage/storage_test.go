package storage_test

// Paginate 的边界行为测试。
//
// Paginate 是纯函数，不依赖 *gorm.DB，因此这里用外部测试包（storage_test）只验证契约：
// page/pageSize 非法时回退默认值，合法时按公式计算 offset；
// 超上限时被夹取（page → MaxPage，pageSize → MaxPageSize），保证不会生成超大 LIMIT / OFFSET。

import (
	"testing"

	"kefu-server/pkg/storage"
)

func TestPaginate(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		pageSize   int
		wantOffset int
		wantLimit  int
	}{
		{
			name: "正常分页：第 1 页每页 20 条",
			page: 1, pageSize: 20,
			wantOffset: 0, wantLimit: 20,
		},
		{
			name: "正常分页：第 3 页每页 10 条",
			page: 3, pageSize: 10,
			wantOffset: 20, wantLimit: 10,
		},
		{
			name: "正常分页：每页 1 条",
			page: 100, pageSize: 1,
			wantOffset: 99, wantLimit: 1,
		},
		{
			name: "page=0 回退为第 1 页",
			page: 0, pageSize: 10,
			wantOffset: 0, wantLimit: 10,
		},
		{
			name: "page 为负数回退为第 1 页",
			page: -7, pageSize: 5,
			wantOffset: 0, wantLimit: 5,
		},
		{
			name: "pageSize=0 回退为默认 20",
			page: 2, pageSize: 0,
			wantOffset: 20, wantLimit: 20,
		},
		{
			name: "pageSize 为负数回退为默认 20",
			page: 2, pageSize: -30,
			wantOffset: 20, wantLimit: 20,
		},
		{
			name: "page 与 pageSize 同时非法：回退为第 1 页 20 条",
			page: 0, pageSize: -1,
			wantOffset: 0, wantLimit: 20,
		},
		{
			// 修复点：pageSize 必须有上限，超大的 query 参数（?page_size=100000000）
			// 会被夹取到 MaxPageSize，不能原样拼进 LIMIT。
			name:       "pageSize 超上限 100000 被夹取到 MaxPageSize(200)",
			page:       1,
			pageSize:   100000,
			wantOffset: 0, wantLimit: storage.MaxPageSize,
		},
		{
			name:       "pageSize 恰好等于上限 200：不做改动",
			page:       1,
			pageSize:   storage.MaxPageSize,
			wantOffset: 0, wantLimit: storage.MaxPageSize,
		},
		{
			name:       "pageSize 超上限且在第 2 页：offset 按夹取后的 limit 计算",
			page:       2,
			pageSize:   100000,
			wantOffset: storage.MaxPageSize, wantLimit: storage.MaxPageSize,
		},
		{
			name:       "page 超上限被夹取到 MaxPage(10000)",
			page:       storage.MaxPage + 1,
			pageSize:   10,
			wantOffset: (storage.MaxPage - 1) * 10, wantLimit: 10,
		},
		{
			name:       "page 极大（1000000）被夹取到 MaxPage，offset 不会无限放大",
			page:       1000000,
			pageSize:   100,
			wantOffset: (storage.MaxPage - 1) * 100, wantLimit: 100,
		},
		{
			name:       "page 与 pageSize 同时超上限：两者都夹取",
			page:       20000,
			pageSize:   5000,
			wantOffset: (storage.MaxPage - 1) * storage.MaxPageSize, wantLimit: storage.MaxPageSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, limit := storage.Paginate(tt.page, tt.pageSize)
			if offset != tt.wantOffset {
				t.Errorf("Paginate(%d, %d) offset = %d, 期望 %d", tt.page, tt.pageSize, offset, tt.wantOffset)
			}
			if limit != tt.wantLimit {
				t.Errorf("Paginate(%d, %d) limit = %d, 期望 %d", tt.page, tt.pageSize, limit, tt.wantLimit)
			}
			// 额外校验不变式：合法范围内 offset 必须是非负的、且是 (page-1)*pageSize 的整数倍
			if offset < 0 {
				t.Errorf("Paginate(%d, %d) offset = %d, 不应为负数", tt.page, tt.pageSize, offset)
			}
			if tt.wantOffset > 0 && tt.wantOffset%tt.wantLimit != 0 {
				t.Errorf("offset=%d 不是 limit=%d 的整数倍，分页错位", offset, limit)
			}
			// 上限不变式：无论调用方传什么，返回的 limit 都不能超过单页上限，
			// offset 也不能超过「最大页码」对应的偏移（DoS 防护的实际断言）。
			if limit > storage.MaxPageSize {
				t.Errorf("Paginate(%d, %d) limit = %d, 超过单页上限 %d",
					tt.page, tt.pageSize, limit, storage.MaxPageSize)
			}
			if offset > (storage.MaxPage-1)*storage.MaxPageSize {
				t.Errorf("Paginate(%d, %d) offset = %d, 超过页码上限对应的最大偏移 %d",
					tt.page, tt.pageSize, offset, (storage.MaxPage-1)*storage.MaxPageSize)
			}
		})
	}
}

// TestPaginateConstants 固定导出常量的取值：
// 默认 20 / 单页 200 / 页码 10000。调用方与前端都按这组数字对齐，改动需同步。
func TestPaginateConstants(t *testing.T) {
	if storage.DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, 期望 20", storage.DefaultPageSize)
	}
	if storage.MaxPageSize != 200 {
		t.Errorf("MaxPageSize = %d, 期望 200（现有前端最大用 200）", storage.MaxPageSize)
	}
	if storage.MaxPage != 10000 {
		t.Errorf("MaxPage = %d, 期望 10000", storage.MaxPage)
	}
}

// TestPaginateOffsetMonotonic 同一页大小下，页码递增时 offset 必须严格递增，
// 保证翻页不会重复或跳页（用表驱动固定若干页，不依赖时间/随机）。
func TestPaginateOffsetMonotonic(t *testing.T) {
	const pageSize = 15
	prev := -1
	for page := 1; page <= 5; page++ {
		offset, limit := storage.Paginate(page, pageSize)
		if limit != pageSize {
			t.Fatalf("第 %d 页 limit = %d, 期望 %d", page, limit, pageSize)
		}
		want := (page - 1) * pageSize
		if offset != want {
			t.Fatalf("第 %d 页 offset = %d, 期望 %d", page, offset, want)
		}
		if offset <= prev {
			t.Fatalf("第 %d 页 offset = %d 未比上一页 %d 递增", page, offset, prev)
		}
		prev = offset
	}
}

// TestClampPage ClampPage 与 Paginate 必须使用同一套夹取规则：
// 响应回显 page/page_size 时用它的返回值，保证「回显值 == 实际生效值」。
func TestClampPage(t *testing.T) {
	tests := []struct {
		name               string
		page, pageSize     int
		wantPage, wantSize int
	}{
		{"正常值原样返回", 3, 50, 3, 50},
		{"page=0 视为第 1 页", 0, 20, 1, 20},
		{"page 负数视为第 1 页", -5, 20, 1, 20},
		{"pageSize=0 用默认值", 2, 0, 2, storage.DefaultPageSize},
		{"pageSize 负数用默认值", 2, -1, 2, storage.DefaultPageSize},
		{"pageSize 超上限被夹取", 1, 100000000, 1, storage.MaxPageSize},
		{"page 超上限被夹取", storage.MaxPage + 1, 20, storage.MaxPage, 20},
		{"边界值不夹取", storage.MaxPage, storage.MaxPageSize, storage.MaxPage, storage.MaxPageSize},
		{"双超限都被夹取", 999999, 999999, storage.MaxPage, storage.MaxPageSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPage, gotSize := storage.ClampPage(tt.page, tt.pageSize)
			if gotPage != tt.wantPage || gotSize != tt.wantSize {
				t.Fatalf("ClampPage(%d,%d) = (%d,%d), 期望 (%d,%d)",
					tt.page, tt.pageSize, gotPage, gotSize, tt.wantPage, tt.wantSize)
			}
			// 与 Paginate 的一致性：offset/limit 必须由同一套夹取后的值算出
			offset, limit := storage.Paginate(tt.page, tt.pageSize)
			if limit != gotSize {
				t.Fatalf("Paginate limit = %d, 与 ClampPage 的 pageSize %d 不一致", limit, gotSize)
			}
			if offset != (gotPage-1)*gotSize {
				t.Fatalf("Paginate offset = %d, 期望 (%d-1)*%d", offset, gotPage, gotSize)
			}
		})
	}
}
