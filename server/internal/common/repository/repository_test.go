package repository_test

// 通知域的数据访问层测试（设备 registration + 站内提醒）。
//
// 数据库：gorm + 纯 Go 以外的方式不引入真实库文件，统一用 sqlite 内存库：
//   - 每个测试函数一个唯一的库名（file:notify-test-N?mode=memory&cache=shared）
//   - SetMaxOpenConns(1)：sqlite 的 cache=shared 内存库在所有连接关闭时销毁，
//     且不同连接可能看到不同的库；限制成单连接才能保证多次查询命中同一个库
//   - 结束时关闭连接，库随之释放，因此测试之间零残留、可乱序 / 重复执行
//
// 说明：repository 的方法都通过 pkg/storage 的全局连接取库，所以每个测试都用 storage.SetDB 注入。

import (
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/internal/common/service"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDBSeq 生成递增的库名，避免同一进程内不同测试复用同一个内存库。
var testDBSeq atomic.Uint64

// newTestDB 建立一个独立的内存库，迁移本包用到的设备表与提醒表，并注入全局连接。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:notify-test-%d?mode=memory&cache=shared", testDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存 sqlite 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	// 内存库 + cache=shared 必须单连接，否则新连接会拿到一个空库。
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.Device{}, &model.Notification{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	storage.Default().SetDB(db)
	return db
}

// ---------------- 测试用装配helper ----------------

// createDevice 写入一台设备，失败即中止（装配阶段，不代表断言）。
func createDevice(t *testing.T, r *repository.DeviceRepository, tenantID, userID uint, name, device, platform string) *model.Device {
	t.Helper()
	d := &model.Device{
		TenantID: tenantID, UserID: userID,
		Device: device, Platform: platform,
		PushToken: "token-" + name, Name: name,
	}
	if err := r.CreateDevice(d); err != nil {
		t.Fatalf("CreateDevice(%s) 失败: %v", name, err)
	}
	if d.ID == 0 {
		t.Fatalf("CreateDevice(%s) 后主键仍为 0，写入未生效", name)
	}
	return d
}

// createNotification 写入一条站内提醒。
func createNotification(t *testing.T, r *repository.NotificationRepository, tenantID, userID uint, title string) *model.Notification {
	t.Helper()
	n := &model.Notification{
		TenantID: tenantID, UserID: userID,
		Type: "conversation.assigned", Title: title, Body: title + "-body",
		ConversationID: 100,
	}
	if err := r.CreateNotification(n); err != nil {
		t.Fatalf("CreateNotification(%s) 失败: %v", title, err)
	}
	if n.ID == 0 {
		t.Fatalf("CreateNotification(%s) 后主键仍为 0，写入未生效", title)
	}
	return n
}

// mustListDevices 取设备列表，失败即中止。
func mustListDevices(t *testing.T, r *repository.DeviceRepository, tenantID, userID uint) []*model.Device {
	t.Helper()
	list, err := r.ListDevicesOfUser(tenantID, userID)
	if err != nil {
		t.Fatalf("ListDevicesOfUser(%d, %d) 失败: %v", tenantID, userID, err)
	}
	return list
}

// mustListNotifications 取增量提醒列表，失败即中止。
func mustListNotifications(t *testing.T, r *repository.NotificationRepository, tenantID, userID, sinceID uint, limit int) []*model.Notification {
	t.Helper()
	list, err := r.ListNotificationsAfter(tenantID, userID, sinceID, limit)
	if err != nil {
		t.Fatalf("ListNotificationsAfter(%d, %d, %d, %d) 失败: %v", tenantID, userID, sinceID, limit, err)
	}
	return list
}

// deviceNames 提取设备名（保持返回顺序），便于与期望列表逐项比较。
func deviceNames(list []*model.Device) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.Name)
	}
	return out
}

// notificationIDs 提取提醒主键（保持返回顺序）。
func notificationIDs(list []*model.Notification) []uint {
	out := make([]uint, 0, len(list))
	for _, n := range list {
		out = append(out, n.ID)
	}
	return out
}

// readFlags 把「主键 → 是否已读」摊平，避免依赖列表顺序。
func readFlags(list []*model.Notification) map[uint]bool {
	out := make(map[uint]bool, len(list))
	for _, n := range list {
		out[n.ID] = n.Read
	}
	return out
}

// ---------------- 设备 CRUD ----------------

func TestRepository_DevicesCreateAndList(t *testing.T) {
	newTestDB(t)
	r := repository.NewDeviceRepository()

	d1 := createDevice(t, r, 1, 7, "u7-a", "app", "ios")
	d2 := createDevice(t, r, 1, 7, "u7-b", "desktop", "windows")
	// 干扰数据：同租户的另一个用户、另一个租户的同一个 user_id
	dOtherUser := createDevice(t, r, 1, 8, "u8-a", "app", "android")
	dOtherTenant := createDevice(t, r, 2, 7, "t2-u7-a", "web", "mac")

	if d2.ID <= d1.ID {
		t.Fatalf("自增主键未递增：d1.ID=%d, d2.ID=%d", d1.ID, d2.ID)
	}
	if dOtherUser.ID == 0 || dOtherTenant.ID == 0 {
		t.Fatalf("干扰数据未写入：otherUser=%d, otherTenant=%d", dOtherUser.ID, dOtherTenant.ID)
	}

	tests := []struct {
		name     string
		tenantID uint
		userID   uint
		want     []string
	}{
		{
			name:     "本租户本用户的设备：按 id 倒序返回两台",
			tenantID: 1, userID: 7,
			want: []string{"u7-b", "u7-a"},
		},
		{
			name:     "同租户其它用户只能看到自己的设备",
			tenantID: 1, userID: 8,
			want: []string{"u8-a"},
		},
		{
			name:     "其它租户的同名 user_id 相互隔离",
			tenantID: 2, userID: 7,
			want: []string{"t2-u7-a"},
		},
		{
			name:     "没有任何设备的用户返回空列表",
			tenantID: 9, userID: 99,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := mustListDevices(t, r, tt.tenantID, tt.userID)
			if got := deviceNames(list); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("设备名列表 = %v, 期望 %v", got, tt.want)
			}
			if len(list) != len(tt.want) {
				t.Fatalf("设备条数 = %d, 期望 %d", len(list), len(tt.want))
			}
			// 返回的每一行都必须属于查询的租户与用户，且推送 token 未被写坏
			for _, d := range list {
				if d.TenantID != tt.tenantID || d.UserID != tt.userID {
					t.Errorf("设备 %q 归属 (tenant=%d, user=%d), 期望 (%d, %d)",
						d.Name, d.TenantID, d.UserID, tt.tenantID, tt.userID)
				}
				if d.PushToken != "token-"+d.Name {
					t.Errorf("设备 %q 的 push_token = %q, 期望 %q", d.Name, d.PushToken, "token-"+d.Name)
				}
				if d.CreatedAt.IsZero() {
					t.Errorf("设备 %q 的 created_at 未写入", d.Name)
				}
			}
			// 倒序不变式：主键必须严格递减
			for i := 1; i < len(list); i++ {
				if list[i-1].ID <= list[i].ID {
					t.Errorf("设备未按 id 倒序：第 %d 台 id=%d, 第 %d 台 id=%d",
						i-1, list[i-1].ID, i, list[i].ID)
				}
			}
		})
	}
}

func TestRepository_DeleteDeviceOfUser(t *testing.T) {
	newTestDB(t)
	r := repository.NewDeviceRepository()

	keep := createDevice(t, r, 1, 7, "u7-keep", "app", "ios")
	remove := createDevice(t, r, 1, 7, "u7-remove", "desktop", "windows")
	otherUser := createDevice(t, r, 1, 8, "u8-own", "app", "android")

	tests := []struct {
		name     string
		tenantID uint
		userID   uint
		id       uint
		// wantPresent 操作后该 id 是否还应能查到：
		// 本人删除自己的设备 → false；越权/不存在 → 保持操作前的状态。
		wantPresent bool
	}{
		{
			name:     "本人删除自己的设备：删除成功",
			tenantID: 1, userID: 7, id: remove.ID,
			wantPresent: false,
		},
		{
			name:     "越权删除：用别人的 user_id 删自己的设备 id，不生效",
			tenantID: 1, userID: 8, id: keep.ID,
			wantPresent: true,
		},
		{
			name:     "越权删除：拿别人的设备 id 用本人身份删，不生效",
			tenantID: 1, userID: 7, id: otherUser.ID,
			wantPresent: true,
		},
		{
			name:     "跨租户删除：租户不匹配，不生效",
			tenantID: 2, userID: 7, id: keep.ID,
			wantPresent: true,
		},
		{
			// 该 id 本来就不存在，删除语句影响 0 行：既不报错也不应删除任何数据
			name:     "删除不存在的 id：无错误且不影响数据",
			tenantID: 1, userID: 7, id: 999999,
			wantPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.DeleteDeviceOfUser(tt.tenantID, tt.userID, tt.id); err != nil {
				t.Fatalf("DeleteDeviceOfUser(%d, %d, %d) 返回错误: %v", tt.tenantID, tt.userID, tt.id, err)
			}

			// 该设备所属的 (tenant, user) 集合里是否还能查到它
			owners := []struct {
				tenantID uint
				userID   uint
			}{
				{1, 7}, {1, 8}, {2, 7},
			}
			found := false
			for _, o := range owners {
				for _, d := range mustListDevices(t, r, o.tenantID, o.userID) {
					if d.ID == tt.id {
						found = true
					}
				}
			}

			if tt.wantPresent && !found {
				t.Errorf("设备 id=%d 期望仍然存在（越权/跨租户删除不应生效），但查不到了", tt.id)
			}
			if !tt.wantPresent && found {
				t.Errorf("设备 id=%d 期望已不存在，但仍能查到", tt.id)
			}

			// 另外两台与本次操作无关的设备必须一直被保留：
			// 库内共有 3 台设备（u7-keep / u7-remove / u8-own），用例 1 之后恒为 2 台。
			total := 0
			for _, o := range owners {
				total += len(mustListDevices(t, r, o.tenantID, o.userID))
			}
			if total != 2 {
				t.Errorf("全部设备总数 = %d, 期望 2（只有 u7-remove 应被删掉）", total)
			}
		})
	}

	// 收尾：只允许真正属于本人的设备被删掉，其余两台仍在
	if got := deviceNames(mustListDevices(t, r, 1, 7)); !reflect.DeepEqual(got, []string{"u7-keep"}) {
		t.Errorf("用户 7 的设备 = %v, 期望仅剩 [u7-keep]", got)
	}
	if got := deviceNames(mustListDevices(t, r, 1, 8)); !reflect.DeepEqual(got, []string{"u8-own"}) {
		t.Errorf("用户 8 的设备 = %v, 期望仍为 [u8-own]", got)
	}
}

// ---------------- 设备注册幂等（应用层 upsert，不建唯一索引） ----------------

// registerDevice 调 Service.RegisterDevice，失败即中止（装配阶段，不代表断言）。
func registerDevice(t *testing.T, s *service.NotifyService, tenantID, userID uint, req dto.DeviceRequest) *dto.DeviceVO {
	t.Helper()
	vo, err := s.RegisterDevice(tenantID, userID, req)
	if err != nil {
		t.Fatalf("RegisterDevice(tenant=%d, user=%d, %+v) 失败: %v", tenantID, userID, req, err)
	}
	if vo == nil {
		t.Fatalf("RegisterDevice(tenant=%d, user=%d, %+v) 返回 nil", tenantID, userID, req)
	}
	if vo.ID == 0 {
		t.Fatalf("RegisterDevice(tenant=%d, user=%d, %+v) 返回的 VO 主键为 0", tenantID, userID, req)
	}
	return vo
}

// TestServiceRegisterDeviceDedupByToken 同一个 push_token 重复注册（APP 每次启动都会调）
// 只能保留 1 行：主键复用、device / platform / name 被更新。
func TestServiceRegisterDeviceDedupByToken(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewNotifyService()
	r := repository.NewDeviceRepository()

	first := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "tok-7-ios", Name: "我的 iPhone",
	})

	list := mustListDevices(t, r, 1, 7)
	if len(list) != 1 {
		t.Fatalf("首次注册后设备条数 = %d, 期望 1", len(list))
	}
	if list[0].ID != first.ID {
		t.Fatalf("首次注册返回的主键 = %d, 库内主键 = %d", first.ID, list[0].ID)
	}
	if list[0].PushToken != "tok-7-ios" || list[0].Name != "我的 iPhone" ||
		list[0].Device != "app" || list[0].Platform != "ios" {
		t.Fatalf("首次注册落库字段不正确: %+v", list[0])
	}

	// 同一个 token 再注册两次：不新增行，只更新可变字段
	second := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "app", Platform: "android", PushToken: "tok-7-ios", Name: "换机后改名",
	})
	if second.ID != first.ID {
		t.Errorf("重复注册返回的主键 = %d, 期望复用原主键 %d", second.ID, first.ID)
	}
	registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "desktop", Platform: "windows", PushToken: "tok-7-ios", Name: "最后的名字",
	})

	list = mustListDevices(t, r, 1, 7)
	if len(list) != 1 {
		t.Fatalf("同 token 重复注册后设备条数 = %d, 期望仍为 1（不能堆积重复行）", len(list))
	}
	got := list[0]
	if got.ID != first.ID {
		t.Errorf("库内主键 = %d, 期望复用原主键 %d", got.ID, first.ID)
	}
	if got.Device != "desktop" || got.Platform != "windows" || got.Name != "最后的名字" {
		t.Errorf("重复注册后的字段 = (device=%q, platform=%q, name=%q), 期望 (desktop, windows, 最后的名字)",
			got.Device, got.Platform, got.Name)
	}
	if got.PushToken != "tok-7-ios" {
		t.Errorf("push_token = %q, 期望保持 tok-7-ios", got.PushToken)
	}

	// 全表条数也必须是 1（确认没有在别的维度偷偷新增）
	var total int64
	if err := db.Model(&model.Device{}).Count(&total).Error; err != nil {
		t.Fatalf("统计设备总数失败: %v", err)
	}
	if total != 1 {
		t.Errorf("devices 表总行数 = %d, 期望 1", total)
	}
}

// TestServiceRegisterDeviceDifferentTokens 不同 token 是不同设备，各自一行。
func TestServiceRegisterDeviceDifferentTokens(t *testing.T) {
	newTestDB(t)
	svc := service.NewNotifyService()
	r := repository.NewDeviceRepository()

	ios := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "tok-ios", Name: "iPhone",
	})
	android := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "app", Platform: "android", PushToken: "tok-android", Name: "Android",
	})
	desktop := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "desktop", Platform: "windows", PushToken: "tok-desktop", Name: "工作电脑",
	})

	ids := map[uint]bool{ios.ID: true, android.ID: true, desktop.ID: true}
	if len(ids) != 3 {
		t.Fatalf("三台设备的主键出现重复：%d, %d, %d", ios.ID, android.ID, desktop.ID)
	}

	list := mustListDevices(t, r, 1, 7)
	if len(list) != 3 {
		t.Fatalf("设备条数 = %d, 期望 3", len(list))
	}
	// 按 id 倒序返回：最后注册的排在最前
	if got, want := deviceNames(list), []string{"工作电脑", "Android", "iPhone"}; !reflect.DeepEqual(got, want) {
		t.Errorf("设备名列表 = %v, 期望 %v", got, want)
	}
	tokens := map[string]bool{}
	for _, d := range list {
		tokens[d.PushToken] = true
	}
	for _, want := range []string{"tok-ios", "tok-android", "tok-desktop"} {
		if !tokens[want] {
			t.Errorf("token %q 未落库，实际 tokens=%v", want, tokens)
		}
	}
}

// TestServiceRegisterDeviceEmptyTokenDedupByKind push_token 为空（PC / Web 端常见）时
// 按 (tenant, user, device, platform) 去重：同一台机器重复注册只保留 1 行、只更新 name，
// 且 push_token 保持为空。
func TestServiceRegisterDeviceEmptyTokenDedupByKind(t *testing.T) {
	newTestDB(t)
	svc := service.NewNotifyService()
	r := repository.NewDeviceRepository()

	first := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "web", Platform: "mac", PushToken: "", Name: "浏览器-旧名字",
	})
	if first.PushToken != "" {
		t.Errorf("空 token 注册后 VO 的 push_token = %q, 期望空串", first.PushToken)
	}

	second := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "web", Platform: "mac", PushToken: "", Name: "浏览器-新名字",
	})
	if second.ID != first.ID {
		t.Errorf("重复注册返回的主键 = %d, 期望复用原主键 %d", second.ID, first.ID)
	}

	list := mustListDevices(t, r, 1, 7)
	if len(list) != 1 {
		t.Fatalf("同 device+platform 重复注册后设备条数 = %d, 期望 1", len(list))
	}
	if list[0].Name != "浏览器-新名字" {
		t.Errorf("name = %q, 期望被更新为「浏览器-新名字」", list[0].Name)
	}
	if list[0].PushToken != "" {
		t.Errorf("push_token = %q, 期望保持为空", list[0].PushToken)
	}

	// 不同平台 / 不同端不算同一台设备，各自新增一行
	registerDevice(t, svc, 1, 7, dto.DeviceRequest{Device: "web", Platform: "windows", Name: "Windows 浏览器"})
	registerDevice(t, svc, 1, 7, dto.DeviceRequest{Device: "desktop", Platform: "mac", Name: "桌面端"})
	if got := len(mustListDevices(t, r, 1, 7)); got != 3 {
		t.Errorf("不同 device/platform 的设备条数 = %d, 期望 3", got)
	}

	// token 非空的注册不受「设备 + 平台」去重影响，走自己的 token 维度
	registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "web", Platform: "mac", PushToken: "tok-web", Name: "带 token 的浏览器",
	})
	if got := len(mustListDevices(t, r, 1, 7)); got != 4 {
		t.Errorf("带 token 的设备条数 = %d, 期望 4（与空 token 那台不是同一维度）", got)
	}
}

// TestServiceRegisterDeviceIsolatedPerUser 去重维度必须带上 tenant_id / user_id：
// 同一个 token 被不同用户注册时互不影响，各自一行。
func TestServiceRegisterDeviceIsolatedPerUser(t *testing.T) {
	newTestDB(t)
	svc := service.NewNotifyService()
	r := repository.NewDeviceRepository()

	u7 := registerDevice(t, svc, 1, 7, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "shared-token", Name: "用户7的手机",
	})
	u8 := registerDevice(t, svc, 1, 8, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "shared-token", Name: "用户8的手机",
	})
	t2u7 := registerDevice(t, svc, 2, 7, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "shared-token", Name: "另一个租户的手机",
	})

	if u7.ID == u8.ID || u7.ID == t2u7.ID || u8.ID == t2u7.ID {
		t.Fatalf("不同用户/租户注册到了同一行：%d, %d, %d", u7.ID, u8.ID, t2u7.ID)
	}

	if got := deviceNames(mustListDevices(t, r, 1, 7)); !reflect.DeepEqual(got, []string{"用户7的手机"}) {
		t.Errorf("用户 7 的设备 = %v, 期望 [用户7的手机]", got)
	}
	if got := deviceNames(mustListDevices(t, r, 1, 8)); !reflect.DeepEqual(got, []string{"用户8的手机"}) {
		t.Errorf("用户 8 的设备 = %v, 期望 [用户8的手机]", got)
	}
	if got := deviceNames(mustListDevices(t, r, 2, 7)); !reflect.DeepEqual(got, []string{"另一个租户的手机"}) {
		t.Errorf("租户 2 用户 7 的设备 = %v, 期望 [另一个租户的手机]", got)
	}

	// 各自再注册一次：只更新自己那一行，别人的行不受影响
	registerDevice(t, svc, 1, 8, dto.DeviceRequest{
		Device: "app", Platform: "ios", PushToken: "shared-token", Name: "用户8改名",
	})
	if got := deviceNames(mustListDevices(t, r, 1, 7)); !reflect.DeepEqual(got, []string{"用户7的手机"}) {
		t.Errorf("用户 8 重复注册后，用户 7 的设备被改动: %v", got)
	}
	if got := deviceNames(mustListDevices(t, r, 1, 8)); !reflect.DeepEqual(got, []string{"用户8改名"}) {
		t.Errorf("用户 8 的设备 = %v, 期望 [用户8改名]", got)
	}
}

// TestRepository_FindDeviceByTokenAndKind 固定两个查询方法的语义：
// 命中返回对应设备，未命中返回 ErrRecordNotFound；SaveDevice 走主键整体更新。
func TestRepository_FindDeviceByTokenAndKind(t *testing.T) {
	newTestDB(t)
	r := repository.NewDeviceRepository()

	target := createDevice(t, r, 1, 7, "u7-ios", "app", "ios")

	byToken, err := r.FindDeviceByToken(1, 7, target.PushToken)
	if err != nil {
		t.Fatalf("FindDeviceByToken 命中失败: %v", err)
	}
	if byToken.ID != target.ID {
		t.Errorf("FindDeviceByToken 返回 id=%d, 期望 %d", byToken.ID, target.ID)
	}

	byKind, err := r.FindDeviceByKind(1, 7, "app", "ios")
	if err != nil {
		t.Fatalf("FindDeviceByKind 命中失败: %v", err)
	}
	if byKind.ID != target.ID {
		t.Errorf("FindDeviceByKind 返回 id=%d, 期望 %d", byKind.ID, target.ID)
	}

	// 未命中：token / kind / 越界的 user 与 tenant 都必须报错（调用方据此新建）
	misses := []struct {
		name string
		call func() (*model.Device, error)
	}{
		{name: "token 不存在", call: func() (*model.Device, error) { return r.FindDeviceByToken(1, 7, "no-such-token") }},
		{name: "platform 不匹配", call: func() (*model.Device, error) { return r.FindDeviceByKind(1, 7, "app", "android") }},
		{name: "device 不匹配", call: func() (*model.Device, error) { return r.FindDeviceByKind(1, 7, "desktop", "ios") }},
		{name: "别的用户", call: func() (*model.Device, error) { return r.FindDeviceByToken(1, 8, target.PushToken) }},
		{name: "别的租户", call: func() (*model.Device, error) { return r.FindDeviceByKind(2, 7, "app", "ios") }},
	}
	for _, m := range misses {
		t.Run("未命中："+m.name, func(t *testing.T) {
			d, err := m.call()
			if err == nil {
				t.Fatalf("期望返回错误（未命中），实际拿到 id=%d", d.ID)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Errorf("错误 = %v, 期望 gorm.ErrRecordNotFound", err)
			}
		})
	}

	// SaveDevice：整体更新后能按主键读回
	target.Name = "改名后的设备"
	target.Platform = "android"
	if err := r.SaveDevice(target); err != nil {
		t.Fatalf("SaveDevice 失败: %v", err)
	}
	after, err := r.FindDeviceByToken(1, 7, target.PushToken)
	if err != nil {
		t.Fatalf("更新后 FindDeviceByToken 失败: %v", err)
	}
	if after.ID != target.ID || after.Name != "改名后的设备" || after.Platform != "android" {
		t.Errorf("更新后的设备 = (id=%d, name=%q, platform=%q), 期望 (%d, 改名后的设备, android)",
			after.ID, after.Name, after.Platform, target.ID)
	}
	if got := len(mustListDevices(t, r, 1, 7)); got != 1 {
		t.Errorf("SaveDevice 不应新增行，设备条数 = %d, 期望 1", got)
	}
}

// ---------------- 站内提醒增量语义 ----------------

func TestRepository_ListNotificationsAfter(t *testing.T) {
	newTestDB(t)
	r := repository.NewNotificationRepository()

	n1 := createNotification(t, r, 1, 7, "第一条")
	n2 := createNotification(t, r, 1, 7, "第二条")
	n3 := createNotification(t, r, 1, 7, "第三条")
	// 干扰数据：同租户别的用户、别的租户同一个用户
	nOtherUser := createNotification(t, r, 1, 8, "别人的提醒")
	nOtherTenant := createNotification(t, r, 2, 7, "别的租户的提醒")

	if !(n1.ID < n2.ID && n2.ID < n3.ID) {
		t.Fatalf("提醒主键未按写入顺序递增：%d, %d, %d", n1.ID, n2.ID, n3.ID)
	}

	tests := []struct {
		name     string
		tenantID uint
		userID   uint
		sinceID  uint
		limit    int
		wantIDs  []uint
	}{
		{
			name:     "sinceID=0：返回该用户全部提醒，按 id 升序",
			tenantID: 1, userID: 7, sinceID: 0, limit: 10,
			wantIDs: []uint{n1.ID, n2.ID, n3.ID},
		},
		{
			name:     "sinceID=第一条：只返回它之后新增的两条",
			tenantID: 1, userID: 7, sinceID: n1.ID, limit: 10,
			wantIDs: []uint{n2.ID, n3.ID},
		},
		{
			name:     "sinceID=最后一条：没有更新内容，返回空",
			tenantID: 1, userID: 7, sinceID: n3.ID, limit: 10,
			wantIDs: []uint{},
		},
		{
			name:     "sinceID 大于最大 id：返回空",
			tenantID: 1, userID: 7, sinceID: n3.ID + 1000, limit: 10,
			wantIDs: []uint{},
		},
		{
			name:     "limit 生效：只取最早的 2 条",
			tenantID: 1, userID: 7, sinceID: 0, limit: 2,
			wantIDs: []uint{n1.ID, n2.ID},
		},
		{
			name:     "用户隔离：看不到同租户其它用户的提醒",
			tenantID: 1, userID: 8, sinceID: 0, limit: 10,
			wantIDs: []uint{nOtherUser.ID},
		},
		{
			name:     "租户隔离：看不到其它租户同一 user_id 的提醒",
			tenantID: 2, userID: 7, sinceID: 0, limit: 10,
			wantIDs: []uint{nOtherTenant.ID},
		},
		{
			name:     "无数据用户返回空列表",
			tenantID: 5, userID: 55, sinceID: 0, limit: 10,
			wantIDs: []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := mustListNotifications(t, r, tt.tenantID, tt.userID, tt.sinceID, tt.limit)
			if got := notificationIDs(list); !reflect.DeepEqual(got, tt.wantIDs) {
				t.Fatalf("返回的提醒 id 列表 = %v, 期望 %v", got, tt.wantIDs)
			}
			for _, n := range list {
				if n.TenantID != tt.tenantID || n.UserID != tt.userID {
					t.Errorf("提醒 id=%d 归属 (tenant=%d, user=%d), 期望 (%d, %d)",
						n.ID, n.TenantID, n.UserID, tt.tenantID, tt.userID)
				}
				if n.ID <= tt.sinceID {
					t.Errorf("提醒 id=%d 未大于 sinceID=%d，增量语义被破坏", n.ID, tt.sinceID)
				}
				if n.Read {
					t.Errorf("新建的提醒 id=%d 已读标记为 true, 期望默认 false", n.ID)
				}
			}
			// 升序不变式
			for i := 1; i < len(list); i++ {
				if list[i-1].ID >= list[i].ID {
					t.Errorf("提醒未按 id 升序：%d 出现在 %d 之前", list[i-1].ID, list[i].ID)
				}
			}
		})
	}
}

func TestRepository_SetNotificationsRead(t *testing.T) {
	newTestDB(t)
	r := repository.NewNotificationRepository()

	n1 := createNotification(t, r, 1, 7, "甲")
	n2 := createNotification(t, r, 1, 7, "乙")
	n3 := createNotification(t, r, 1, 7, "丙")
	nOtherUser := createNotification(t, r, 1, 8, "别人的提醒")

	tests := []struct {
		name    string
		ids     []uint
		wantMap map[uint]bool // 用户 7 全部提醒的期望已读状态
	}{
		{
			name:    "只标记指定的一条",
			ids:     []uint{n2.ID},
			wantMap: map[uint]bool{n1.ID: false, n2.ID: true, n3.ID: false},
		},
		{
			name:    "标记多条",
			ids:     []uint{n1.ID, n3.ID},
			wantMap: map[uint]bool{n1.ID: true, n2.ID: true, n3.ID: true},
		},
		{
			name:    "ids 为空切片表示全部已读（此时已全读，状态不变）",
			ids:     []uint{},
			wantMap: map[uint]bool{n1.ID: true, n2.ID: true, n3.ID: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.SetNotificationsRead(1, 7, tt.ids); err != nil {
				t.Fatalf("SetNotificationsRead(1, 7, %v) 返回错误: %v", tt.ids, err)
			}

			list := mustListNotifications(t, r, 1, 7, 0, 10)
			if len(list) != 3 {
				t.Fatalf("用户 7 的提醒条数 = %d, 期望 3", len(list))
			}
			if got := readFlags(list); !reflect.DeepEqual(got, tt.wantMap) {
				t.Fatalf("已读状态 = %v, 期望 %v", got, tt.wantMap)
			}

			// 越权保护：别人的提醒不能被顺手标记为已读
			other := mustListNotifications(t, r, 1, 8, 0, 10)
			if len(other) != 1 || other[0].ID != nOtherUser.ID {
				t.Fatalf("用户 8 的提醒列表异常: %v", notificationIDs(other))
			}
			if other[0].Read {
				t.Errorf("用户 8 的提醒被误标记为已读，租户内用户隔离被破坏")
			}
		})
	}

	// 越权：用别人的提醒 id 去标记，不应该影响任何人
	t.Run("ids 属于别的用户时既不越权也不报错", func(t *testing.T) {
		if err := r.SetNotificationsRead(1, 7, []uint{nOtherUser.ID}); err != nil {
			t.Fatalf("SetNotificationsRead 返回错误: %v", err)
		}
		if got := readFlags(mustListNotifications(t, r, 1, 8, 0, 10)); got[nOtherUser.ID] {
			t.Errorf("用户 8 的提醒被用户 7 标记为已读，越权写入")
		}
		if got := readFlags(mustListNotifications(t, r, 1, 7, 0, 10)); !got[n1.ID] || !got[n2.ID] || !got[n3.ID] {
			t.Errorf("用户 7 的已读状态被误改: %v", got)
		}
	})
}

// TestRepository_SetNotificationsReadEmptyIDsMarksAll 单独验证「ids 为空 = 全部已读」，
// 从全未读的干净数据出发，避免被上一个用例的状态掩盖。
func TestRepository_SetNotificationsReadEmptyIDsMarksAll(t *testing.T) {
	newTestDB(t)
	r := repository.NewNotificationRepository()

	a := createNotification(t, r, 1, 7, "a")
	b := createNotification(t, r, 1, 7, "b")
	other := createNotification(t, r, 1, 8, "别人的提醒")

	before := readFlags(mustListNotifications(t, r, 1, 7, 0, 10))
	if before[a.ID] || before[b.ID] {
		t.Fatalf("初始状态应为全未读，实际 %v", before)
	}

	// nil 与空切片都应表示「全部已读」
	for _, ids := range [][]uint{nil, {}} {
		if err := r.SetNotificationsRead(1, 7, ids); err != nil {
			t.Fatalf("SetNotificationsRead(ids=%v) 返回错误: %v", ids, err)
		}
		got := readFlags(mustListNotifications(t, r, 1, 7, 0, 10))
		if len(got) != 2 {
			t.Fatalf("ids=%v 后提醒条数 = %d, 期望 2", ids, len(got))
		}
		if !got[a.ID] || !got[b.ID] {
			t.Errorf("ids=%v 后用户 7 的提醒未全部标记为已读: %v", ids, got)
		}
	}

	// 全部已读只作用在 (1, 7) 范围内
	if got := readFlags(mustListNotifications(t, r, 1, 8, 0, 10)); got[other.ID] {
		t.Errorf("ids 为空时把别的用户（1,8）的提醒也标记成已读了")
	}
}

// TestRepository_ListNotificationsAfterLimitFallback limit <= 0 时必须按默认上限兜底：
// gorm 的 Limit(0) 会生成 LIMIT 0（永远查不到数据），负数等于不写 LIMIT（全表拉取），
// 两者都不是「断线重连后增量补拉」的语义。
func TestRepository_ListNotificationsAfterLimitFallback(t *testing.T) {
	newTestDB(t)
	r := repository.NewNotificationRepository()

	// 造 205 条，足以区分「默认上限 200」与「全量 / 空结果」
	const total = repository.DefaultNotificationLimit + 5
	for i := 0; i < total; i++ {
		createNotification(t, r, 1, 7, fmt.Sprintf("提醒-%03d", i))
	}

	full := mustListNotifications(t, r, 1, 7, 0, total)
	if len(full) != total {
		t.Fatalf("全量拉取条数 = %d, 期望 %d", len(full), total)
	}
	wantIDs := notificationIDs(full[:repository.DefaultNotificationLimit])

	for _, limit := range []int{0, -1, -200} {
		t.Run(fmt.Sprintf("limit=%d 按默认上限 %d 处理", limit, repository.DefaultNotificationLimit), func(t *testing.T) {
			list := mustListNotifications(t, r, 1, 7, 0, limit)
			if len(list) != repository.DefaultNotificationLimit {
				t.Fatalf("返回条数 = %d, 期望 %d（既不能是 0，也不能是全量 %d）",
					len(list), repository.DefaultNotificationLimit, total)
			}
			if got := notificationIDs(list); !reflect.DeepEqual(got, wantIDs) {
				t.Errorf("返回的提醒 id 列表与「最早的 %d 条」不一致", repository.DefaultNotificationLimit)
			}
		})
	}

	// 正数 limit 仍然照常生效（兜底不影响正常调用）
	if got := len(mustListNotifications(t, r, 1, 7, 0, 3)); got != 3 {
		t.Errorf("limit=3 时返回条数 = %d, 期望 3", got)
	}
	if got := len(mustListNotifications(t, r, 1, 7, 0, repository.DefaultNotificationLimit+1)); got != repository.DefaultNotificationLimit+1 {
		t.Errorf("limit=%d 时返回条数 = %d, 期望 %d",
			repository.DefaultNotificationLimit+1, got, repository.DefaultNotificationLimit+1)
	}
}

// TestRepository_NotificationsSurviveReconnect 模拟端侧断线重连：
// 第一次同步到 n2 为止，离线期间新增 n3，重连后 sinceID=n2 必须恰好拿到新增的那条。
func TestRepository_NotificationsSurviveReconnect(t *testing.T) {
	newTestDB(t)
	r := repository.NewNotificationRepository()

	createNotification(t, r, 1, 7, "在线时收到-1")
	n2 := createNotification(t, r, 1, 7, "在线时收到-2")

	firstSync := mustListNotifications(t, r, 1, 7, 0, 100)
	if len(firstSync) != 2 {
		t.Fatalf("首次全量同步条数 = %d, 期望 2", len(firstSync))
	}
	lastSeen := firstSync[len(firstSync)-1].ID
	if lastSeen != n2.ID {
		t.Fatalf("首次同步的最大 id = %d, 期望 %d", lastSeen, n2.ID)
	}

	// 离线期间新增两条
	offline1 := createNotification(t, r, 1, 7, "离线-1")
	offline2 := createNotification(t, r, 1, 7, "离线-2")

	incremental := mustListNotifications(t, r, 1, 7, lastSeen, 100)
	if got, want := notificationIDs(incremental), []uint{offline1.ID, offline2.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("重连增量同步 = %v, 期望 %v", got, want)
	}

	// 再同步一次：以本次最大 id 为游标，不应重复下发
	next := mustListNotifications(t, r, 1, 7, incremental[len(incremental)-1].ID, 100)
	if len(next) != 0 {
		t.Errorf("没有新提醒时仍然返回了 %v", notificationIDs(next))
	}
}
