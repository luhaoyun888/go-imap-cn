package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleLogin 处理登录请求。
// 参数：
//
//	tag - 请求的标签
//	dec - 解码器，用于解析请求数据
//
// 返回：错误信息，如果有的话
func (c *Conn) handleLogin(tag string, dec *imapwire.Decoder) error {
	var username, password string
	// 检查参数格式是否正确
	if !dec.ExpectSP() || !dec.ExpectAString(&username) || !dec.ExpectSP() || !dec.ExpectAString(&password) || !dec.ExpectCRLF() {
		return dec.Err() // 返回解码错误
	}

	// 检查连接状态是否未认证
	if err := c.checkState(imap.ConnStateNotAuthenticated); err != nil {
		return err // 返回状态检查错误
	}

	// 检查是否可以进行认证
	if !c.canAuth() {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodePrivacyRequired,
			Text: "需要使用 TLS 进行身份验证",
		}
	}

	// 执行登录操作
	if err := c.session.Login(username, password); err != nil {
		return err // 返回登录错误
	}

	// 更新连接状态为已认证
	c.state = imap.ConnStateAuthenticated
	// 返回成功状态和信息
	return c.writeCapabilityStatus(tag, imap.StatusResponseTypeOK, "登录成功") // 替换为中文
}
