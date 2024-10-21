package imapserver

import (
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// appendLimit 是 APPEND 有效负载的最大大小。
const appendLimit = 100 * 1024 * 1024 // 100MiB

// handleAppend 处理 APPEND 命令。
// tag: 客户端提供的标记，dec: 用于解码请求的 Decoder。
func (c *Conn) handleAppend(tag string, dec *imapwire.Decoder) error {
	var (
		mailbox string             // 邮箱名称
		options imap.AppendOptions // 附加选项
	)

	// 解析请求，期望空格后跟邮箱名称
	if !dec.ExpectSP() || !dec.ExpectMailbox(&mailbox) || !dec.ExpectSP() {
		return dec.Err() // 返回解析错误
	}

	// 解析标志列表
	hasFlagList, err := dec.List(func() error {
		flag, err := internal.ExpectFlag(dec) // 期望标志
		if err != nil {
			return err // 返回错误
		}
		options.Flags = append(options.Flags, flag) // 添加标志到选项中
		return nil
	})
	if err != nil {
		return err // 返回错误
	}
	if hasFlagList && !dec.ExpectSP() {
		return dec.Err() // 返回解析错误
	}

	// 解析时间
	t, err := internal.DecodeDateTime(dec) // 解析日期时间
	if err != nil {
		return err // 返回错误
	}
	if !t.IsZero() && !dec.ExpectSP() {
		return dec.Err() // 返回解析错误
	}
	options.Time = t // 设置时间选项

	var dataExt string      // 数据扩展
	if dec.Atom(&dataExt) { // 如果存在数据扩展
		switch strings.ToUpper(dataExt) { // 转换为大写进行匹配
		case "UTF8":
			// '~' 是 literal8 前缀
			if !dec.ExpectSP() || !dec.ExpectSpecial('(') || !dec.ExpectSpecial('~') {
				return dec.Err() // 返回解析错误
			}
		default:
			return newClientBugError("未知的 APPEND 数据扩展") // 返回未知扩展错误
		}
	} else {
		dec.Special('~') // 如果存在 BINARY，则忽略 literal8 前缀
	}

	// 解析邮件内容
	lit, nonSync, err := dec.ExpectLiteralReader() // 期望字面量读取器
	if err != nil {
		return err // 返回错误
	}

	// 检查字面量大小是否超出限制
	if lit.Size() > appendLimit {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTooBig,
			Text: fmt.Sprintf("字面量大小限制为 %v 字节", appendLimit),
		}
	}
	if err := c.acceptLiteral(lit.Size(), nonSync); err != nil {
		return err // 返回错误
	}

	c.setReadTimeout(literalReadTimeout)   // 设置读取超时
	defer c.setReadTimeout(cmdReadTimeout) // 恢复读取超时

	// 检查连接状态是否为已认证
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		io.Copy(io.Discard, lit) // 读取并丢弃邮件内容
		dec.CRLF()               // 读取 CRLF
		return err               // 返回错误
	}

	// 调用会话的 Append 方法
	data, appendErr := c.session.Append(mailbox, lit, &options)
	if _, discardErr := io.Copy(io.Discard, lit); discardErr != nil {
		return err // 返回错误
	}
	if dataExt != "" && !dec.ExpectSpecial(')') {
		return dec.Err() // 返回解析错误
	}
	if !dec.ExpectCRLF() {
		return err // 返回错误
	}
	if appendErr != nil {
		return appendErr // 返回附加错误
	}
	if err := c.poll("APPEND"); err != nil {
		return err // 返回错误
	}
	return c.writeAppendOK(tag, data) // 返回 APPEND 完成响应
}

// writeAppendOK 写入 APPEND 成功的响应。
// tag: 客户端提供的标记，data: 附加的数据。
func (c *Conn) writeAppendOK(tag string, data *imap.AppendData) error {
	enc := newResponseEncoder(c) // 创建响应编码器
	defer enc.end()              // 确保结束编码

	enc.Atom(tag).SP().Atom("OK").SP() // 编码标记和 OK 响应
	if data != nil {
		enc.Special('[')
		enc.Atom("APPENDUID").SP().Number(data.UIDValidity).SP().UID(data.UID) // 编码 UID 信息
		enc.Special(']').SP()
	}
	enc.Text("APPEND 完成") // 编码完成消息
	return enc.CRLF()     // 返回 CRLF
}
