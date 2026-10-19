package realtime

import (
	"fmt"
	"log"

	"kefu-server/internal/common/config"
	"kefu-server/internal/common/model"
)

// ---------------- 厂商推送网关 ----------------
// 职责：当客服离线时，把提醒通过厂商通道（APNs / 极光 / FCM ...）下发到 APP。
// 在线时走 WebSocket（/ws）实时下发，无需厂商推送。
// 完整对接说明见 docs/推送对接.md

// Provider 一个厂商推送通道。实现它即可接入任意厂商。
type Provider interface {
	// Name 通道名，用于日志
	Name() string
	// Push 向单个设备下发一条提醒
	Push(d *model.Device, n *model.Notification) error
}

// PushGateway 推送网关：按设备平台路由到对应厂商通道
type PushGateway struct {
	providers map[string]Provider // key: device.platform（ios / android / ...）
}

var pushGateway = NewPushGateway()

// NewPushGateway 创建网关并注册已配置的厂商通道。
// 未在 config.yaml 启用的厂商不会被注册，其设备自动跳过（仅依赖 WebSocket + 离线补拉）。
func NewPushGateway() *PushGateway {
	p := &PushGateway{providers: map[string]Provider{}}

	// ↓↓↓ 在这里注册厂商通道（详见 docs/推送对接.md 第 4 节）↓↓↓
	//
	// 例：APNs（iOS），需先 go get github.com/sideshow/apns2
	// if cfg := config.C.Push.APNs; cfg.Enabled && cfg.KeyFile != "" {
	//     prov, err := NewAPNsProvider(cfg.KeyID, cfg.TeamID, cfg.BundleID, cfg.KeyFile, cfg.Production)
	//     if err != nil { log.Printf("[push] APNs 初始化失败: %v", err) } else { p.providers["ios"] = prov }
	// }
	//
	// 例：极光（Android / iOS 国内），需先 go get github.com/jpush/jpush-api-go-client
	// if cfg := config.C.Push.JPush; cfg.Enabled && cfg.AppKey != "" {
	//     p.providers["android"] = NewJPushProvider(cfg.AppKey, cfg.MasterSecret)
	//     p.providers["ios"] = NewJPushProvider(cfg.AppKey, cfg.MasterSecret)  // 若走极光通道下发 iOS
	// }

	return p
}

// Register 手动注册一个厂商通道（测试或动态启停用）
func (p *PushGateway) Register(platform string, prov Provider) {
	p.providers[platform] = prov
}

// InitPush 初始化推送网关（启动时调用，注册已配置的厂商通道）
func InitPush() {
	gw := NewPushGateway()
	pushGateway = gw
	if !gw.Enabled() {
		log.Println("[push] 厂商推送未启用（config.yaml: push.enabled），仅记录日志；" +
			"离线提醒仍会落库，端侧重连后可通过 /api/offline/notifications 拉取")
		return
	}
	if len(gw.providers) == 0 {
		log.Println("[push] 已启用推送但未注册任何厂商通道，请在 platform/realtime/push.go 的 NewPushGateway 中注册 " +
			"（详见 docs/推送对接.md）")
	}
}

// Enabled 推送总开关。未开启时只记录日志，不实际下发。
func (p *PushGateway) Enabled() bool { return config.C.Push.Enabled }

// Send 向某用户的所有已注册设备下发一条提醒。
// 调用入口统一在 platform/realtime/notify.go 的 Notify()，业务代码无需关心。
func (p *PushGateway) Send(tenantID, userID uint, n *model.Notification) {
	// 设备列表由 common/service 提供（init 时经 SetDeviceLister 注册），
	// 避免 platform 与 core 各维护一份相同 SQL。
	if deviceLister == nil {
		return
	}
	devices, err := deviceLister(tenantID, userID)
	if err != nil {
		log.Printf("[push] 查询设备失败: %v", err)
		return
	}
	if len(devices) == 0 {
		// 没有注册设备 → 用户完全离线。提醒已落库，
		// 等其重连时由 GET /api/offline/notifications 拉取。
		return
	}

	for _, d := range devices {
		if d.PushToken == "" {
			continue // 该设备没配推送 Token（如 PC 桌面端），只走 WebSocket
		}
		prov, ok := p.providers[d.Platform]
		if !ok {
			// 该平台未接厂商推送：仅记录，不报错（端侧仍可通过离线补拉拿到提醒）
			if p.Enabled() {
				log.Printf("[push] 跳过 platform=%s device=%d：未注册该平台厂商通道", d.Platform, d.ID)
			}
			continue
		}
		if err := prov.Push(d, n); err != nil {
			log.Printf("[push] %s 下发失败 device=%d token=%s: %v",
				prov.Name(), d.ID, mask(d.PushToken), err)
			// TODO: 厂商返回 token 失效（未注册/非法 token）时，可清理该设备：
			//   storage.Default().DB().Where("id = ?", d.ID).Delete(&model.Device{})
			continue
		}
		log.Printf("[push] %s 下发成功 device=%d platform=%s title=%q",
			prov.Name(), d.ID, d.Platform, n.Title)
	}
}

// mask 日志脱敏，避免把完整 Token 写进日志
func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return fmt.Sprintf("%s***%s", s[:4], s[len(s)-4:])
}
