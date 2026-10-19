package storage_test

// Paginate 的边界行为测试。
//
// Paginate 是纯函数，不依赖 *gorm.DB，因此这里用外部测试包（storage_test）只验证契约：
// page/pageSize 非法时回退默认值，合法时按公式计算 offset。

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
			// 注意：当前实现没有 pageSize 上限，超大值会被原样返回（见汇报中的可疑点）。
			name:       "pageSize 超上限 100000 原样返回（当前实现无上限）",
			page:       1,
			pageSize:   100000,
			wantOffset: 0, wantLimit: 100000,
		},
		{
			name: "大页码线性放大 offset",
			page: 1000000, pageSize: 100,
			wantOffset: 99999900, wantLimit: 100,
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
		})
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
