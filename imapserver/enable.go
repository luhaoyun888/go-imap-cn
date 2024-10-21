package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleEnable 处理 ENABLE 命令，启用客户端请求的能力。
// 参数：
//
//	dec: 用于解码请求的 imapwire 解码器。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) handleEnable(dec *imapwire.Decoder) error {
	var requested []imap.Cap // 存储客户端请求的能力
	// 解析客户端请求的能力
	for dec.SP() {
		var c string
		if !dec.ExpectAtom(&c) {
			return dec.Err() // 如果解析失败，返回错误信息
		}
		requested = append(requested, imap.Cap(c)) // 将能力添加到请求列表
	}
	if !dec.ExpectCRLF() {
		return dec.Err() // 确保命令以 CRLF 结束
	}

	// 检查连接状态是否为已认证
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 返回错误信息
	}

	var enabled []imap.Cap // 存储启用的能力
	// 检查请求的能力是否可以启用
	for _, req := range requested {
		switch req {
		case imap.CapIMAP4rev2, imap.CapUTF8Accept:
			enabled = append(enabled, req) // 启用请求的能力
		}
	}

	c.mutex.Lock() // 加锁以保护对启用能力的修改
	for _, e := range enabled {
		c.enabled[e] = struct{}{} // 将能力标记为已启用
	}
	c.mutex.Unlock() // 解锁

	enc := newResponseEncoder(c)       // 创建响应编码器
	defer enc.end()                    // 确保在函数结束时结束编码
	enc.Atom("*").SP().Atom("ENABLED") // 编码启用能力的响应
	for _, c := range enabled {
		enc.SP().Atom(string(c)) // 添加每个已启用的能力
	}
	return enc.CRLF() // 返回编码后的响应
}
