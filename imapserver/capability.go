package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleCapability 处理 CAPABILITY 命令。
// dec: 用于解码请求的 Decoder。
func (c *Conn) handleCapability(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 返回解析错误
	}

	enc := newResponseEncoder(c)          // 创建响应编码器
	defer enc.end()                       // 确保结束编码
	enc.Atom("*").SP().Atom("CAPABILITY") // 写入响应头
	for _, c := range c.availableCaps() { // 遍历可用能力
		enc.SP().Atom(string(c)) // 添加能力到响应中
	}
	return enc.CRLF() // 返回结束标记
}

// availableCaps 返回服务器支持的能力。
// 它们依赖于连接状态。
// 一些扩展（例如 SASL-IR、ENABLE）不需要后端支持，因此总是启用。
func (c *Conn) availableCaps() []imap.Cap {
	available := c.server.options.caps() // 获取服务器可用的能力

	var caps []imap.Cap // 存储能力的切片
	// 添加 IMAP 的基本能力
	addAvailableCaps(&caps, available, []imap.Cap{
		imap.CapIMAP4rev2,
		imap.CapIMAP4rev1,
	})
	if len(caps) == 0 {
		panic("imapserver: 必须支持至少 IMAP4rev1 或 IMAP4rev2") // 确保支持至少一种 IMAP 版本
	}

	// 根据可用能力和状态添加其他能力
	if available.Has(imap.CapIMAP4rev1) {
		caps = append(caps, []imap.Cap{
			imap.CapSASLIR,
			imap.CapLiteralMinus,
		}...)
	}
	if c.canStartTLS() {
		caps = append(caps, imap.CapStartTLS) // 如果可以启动 TLS，添加能力
	}
	if c.canAuth() { // 如果可以进行身份验证
		mechs := []string{"PLAIN"} // 默认身份验证机制
		if authSess, ok := c.session.(SessionSASL); ok {
			mechs = authSess.AuthenticateMechanisms() // 获取可用的 SASL 机制
		}
		for _, mech := range mechs {
			caps = append(caps, imap.Cap("AUTH="+mech)) // 添加身份验证能力
		}
	} else if c.state == imap.ConnStateNotAuthenticated {
		caps = append(caps, imap.CapLoginDisabled) // 未认证状态下禁用登录能力
	}
	if c.state == imap.ConnStateAuthenticated || c.state == imap.ConnStateSelected {
		if available.Has(imap.CapIMAP4rev1) {
			caps = append(caps, []imap.Cap{
				imap.CapUnselect,
				imap.CapEnable,
				imap.CapIdle,
				imap.CapUTF8Accept,
			}...)
			// 添加其他能力
			addAvailableCaps(&caps, available, []imap.Cap{
				imap.CapNamespace,
				imap.CapUIDPlus,
				imap.CapESearch,
				imap.CapSearchRes,
				imap.CapListExtended,
				imap.CapListStatus,
				imap.CapMove,
				imap.CapStatusSize,
				imap.CapBinary,
			})
		}
		// 添加其他能力
		addAvailableCaps(&caps, available, []imap.Cap{
			imap.CapCreateSpecialUse,
			imap.CapLiteralPlus,
			imap.CapUnauthenticate,
		})
	}
	return caps // 返回可用能力
}

// addAvailableCaps 将可用的能力添加到 caps 切片中。
// caps: 目标切片，available: 可用能力集合，l: 要添加的能力列表。
func addAvailableCaps(caps *[]imap.Cap, available imap.CapSet, l []imap.Cap) {
	for _, c := range l {
		if available.Has(c) {
			*caps = append(*caps, c) // 如果可用，添加能力
		}
	}
}
