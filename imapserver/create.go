package imapserver

import (
	"strings"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleCreate 处理 CREATE 命令，创建一个新的邮箱。
// 参数：
//
//	dec: 用于解码请求的 imapwire 解码器。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) handleCreate(dec *imapwire.Decoder) error {
	var (
		name    string             // 存储邮箱名称
		options imap.CreateOptions // 存储创建邮箱的选项
	)

	// 解析邮箱名称
	if !dec.ExpectSP() || !dec.ExpectMailbox(&name) {
		return dec.Err() // 如果解析失败，返回错误信息
	}

	// 解析可选参数
	if dec.SP() {
		var name string
		if !dec.ExpectSpecial('(') || !dec.ExpectAtom(&name) || !dec.ExpectSP() {
			return dec.Err() // 如果解析失败，返回错误信息
		}

		// 根据参数名称设置选项
		switch strings.ToUpper(name) {
		case "USE":
			var err error
			options.SpecialUse, err = internal.ExpectMailboxAttrList(dec) // 解析特殊用途属性
			if err != nil {
				return err // 如果解析失败，返回错误信息
			}
		default:
			return newClientBugError("未知的 CREATE 参数") // 返回未知参数错误
		}

		// 确保参数闭合
		if !dec.ExpectSpecial(')') {
			return dec.Err() // 如果解析失败，返回错误信息
		}
	}

	// 确保命令以 CRLF 结束
	if !dec.ExpectCRLF() {
		return dec.Err() // 如果解析失败，返回错误信息
	}

	// 检查连接状态是否为已认证
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 返回错误信息
	}

	// 创建新的邮箱
	return c.session.Create(name, &options) // 返回创建操作的结果
}
