package imapserver

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
	"github.com/emersion/go-sasl"
)

// handleAuthenticate 处理 AUTHENTICATE 命令。
// tag: 客户端提供的标记，dec: 用于解码请求的 Decoder。
func (c *Conn) handleAuthenticate(tag string, dec *imapwire.Decoder) error {
	var mech string // SASL 机制
	if !dec.ExpectSP() || !dec.ExpectAtom(&mech) {
		return dec.Err() // 返回解析错误
	}
	mech = strings.ToUpper(mech) // 将机制转换为大写

	var initialResp []byte // 初始响应
	if dec.SP() {
		var initialRespStr string
		if !dec.ExpectText(&initialRespStr) {
			return dec.Err() // 返回解析错误
		}
		var err error
		initialResp, err = internal.DecodeSASL(initialRespStr) // 解码 SASL 响应
		if err != nil {
			return err // 返回错误
		}
	}

	if !dec.ExpectCRLF() {
		return dec.Err() // 返回解析错误
	}

	if err := c.checkState(imap.ConnStateNotAuthenticated); err != nil {
		return err // 返回错误
	}
	if !c.canAuth() {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodePrivacyRequired,
			Text: "必须使用 TLS 才能进行身份验证", // 身份验证需要 TLS
		}
	}

	var saslServer sasl.Server // SASL 服务器
	if authSess, ok := c.session.(SessionSASL); ok {
		var err error
		saslServer, err = authSess.Authenticate(mech) // 从会话获取 SASL 服务器
		if err != nil {
			return err // 返回错误
		}
	} else {
		if mech != "PLAIN" { // 如果机制不支持
			return &imap.Error{
				Type: imap.StatusResponseTypeNo,
				Text: "不支持的 SASL 机制", // 返回不支持的错误
			}
		}
		saslServer = sasl.NewPlainServer(func(identity, username, password string) error {
			if identity != "" && identity != username { // 验证身份
				return &imap.Error{
					Type: imap.StatusResponseTypeNo,
					Code: imap.ResponseCodeAuthorizationFailed,
					Text: "不支持的 SASL 身份", // 身份不匹配
				}
			}
			return c.session.Login(username, password) // 进行登录
		})
	}

	enc := newResponseEncoder(c) // 创建响应编码器
	defer enc.end()              // 确保结束编码

	resp := initialResp // 使用初始响应
	for {
		// 获取挑战信息
		challenge, done, err := saslServer.Next(resp)
		if err != nil {
			return err // 返回错误
		} else if done {
			break // 完成身份验证
		}

		var challengeStr string
		if challenge != nil {
			challengeStr = internal.EncodeSASL(challenge) // 编码挑战
		}
		if err := writeContReq(enc.Encoder, challengeStr); err != nil {
			return err // 返回错误
		}

		// 读取客户端响应
		encodedResp, isPrefix, err := c.br.ReadLine()
		if err != nil {
			return err // 返回错误
		} else if isPrefix {
			return fmt.Errorf("SASL 响应过长") // 返回响应过长错误
		} else if string(encodedResp) == "*" {
			return &imap.Error{
				Type: imap.StatusResponseTypeBad,
				Text: "AUTHENTICATE 已取消", // 返回取消错误
			}
		}

		resp, err = decodeSASL(string(encodedResp)) // 解码 SASL 响应
		if err != nil {
			return err // 返回错误
		}
	}

	c.state = imap.ConnStateAuthenticated                               // 设置连接状态为已认证
	text := fmt.Sprintf("%v 身份验证成功", mech)                              // 成功消息
	return writeCapabilityOK(enc.Encoder, tag, c.availableCaps(), text) // 返回成功响应
}

// decodeSASL 解码 SASL 响应字符串。
// s: 要解码的字符串。
func decodeSASL(s string) ([]byte, error) {
	b, err := internal.DecodeSASL(s) // 解码 SASL
	if err != nil {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "格式错误的 SASL 响应", // 返回格式错误
		}
	}
	return b, nil
}

// handleUnauthenticate 处理 UNAUTHENTICATE 命令。
// dec: 用于解码请求的 Decoder。
func (c *Conn) handleUnauthenticate(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 返回解析错误
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 返回错误
	}
	session, ok := c.session.(SessionUnauthenticate) // 检查会话是否支持 UNAUTHENTICATE
	if !ok {
		return newClientBugError("UNAUTHENTICATE 不支持") // 返回不支持的错误
	}
	if err := session.Unauthenticate(); err != nil {
		return err // 返回错误
	}
	c.state = imap.ConnStateNotAuthenticated // 设置连接状态为未认证
	c.mutex.Lock()                           // 锁定互斥量
	c.enabled = make(imap.CapSet)            // 清空可用能力集
	c.mutex.Unlock()                         // 解锁
	return nil                               // 返回成功
}
