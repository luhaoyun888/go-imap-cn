package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleExpunge 处理 EXPUNGE 命令，删除已标记为删除的邮件。
// 参数：
//
//	dec: 用于解码请求的 imapwire 解码器。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) handleExpunge(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 确保命令以 CRLF 结束
	}
	return c.expunge(nil) // 调用 expunge 函数处理删除操作
}

// handleUIDExpunge 处理 UID EXPUNGE 命令，删除指定 UID 的邮件。
// 参数：
//
//	dec: 用于解码请求的 imapwire 解码器。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) handleUIDExpunge(dec *imapwire.Decoder) error {
	var uidSet imap.UIDSet // 存储 UID 集合
	if !dec.ExpectSP() || !dec.ExpectUIDSet(&uidSet) || !dec.ExpectCRLF() {
		return dec.Err() // 如果解析失败，返回错误信息
	}
	return c.expunge(&uidSet) // 调用 expunge 函数处理删除操作
}

// expunge 删除指定 UID 的邮件。
// 参数：
//
//	uids: 要删除的 UID 集合，如果为 nil，则删除所有已标记为删除的邮件。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) expunge(uids *imap.UIDSet) error {
	if err := c.checkState(imap.ConnStateSelected); err != nil {
		return err // 检查连接状态是否为已选择，返回错误信息
	}
	w := &ExpungeWriter{conn: c}      // 创建 ExpungeWriter 实例
	return c.session.Expunge(w, uids) // 调用会话的 Expunge 方法执行删除
}

// writeExpunge 写入 EXPUNGE 更新响应。
// 参数：
//
//	seqNum: 被删除邮件的序列号。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) writeExpunge(seqNum uint32) error {
	enc := newResponseEncoder(c)                           // 创建响应编码器
	defer enc.end()                                        // 确保在函数结束时结束编码
	enc.Atom("*").SP().Number(seqNum).SP().Atom("EXPUNGE") // 编码 EXPUNGE 响应
	return enc.CRLF()                                      // 返回编码后的响应
}

// ExpungeWriter 写入 EXPUNGE 更新的结构体。
type ExpungeWriter struct {
	conn *Conn // 连接实例
}

// WriteExpunge 通知客户端指定序列号的邮件已被删除。
// 参数：
//
//	seqNum: 被删除邮件的序列号。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (w *ExpungeWriter) WriteExpunge(seqNum uint32) error {
	if w.conn == nil {
		return nil // 如果连接为 nil，直接返回
	}
	return w.conn.writeExpunge(seqNum) // 调用连接的 writeExpunge 方法
}
