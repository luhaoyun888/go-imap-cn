package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleMove 处理移动邮件的请求。
// 参数：
//
//	dec - 解码器，用于解析请求数据
//	numKind - 邮件编号类型
//
// 返回：错误信息，如果有的话
func (c *Conn) handleMove(dec *imapwire.Decoder, numKind NumKind) error {
	numSet, dest, err := readCopy(numKind, dec) // 读取移动的邮件编号和目标
	if err != nil {
		return err // 返回读取错误
	}

	// 检查连接状态是否为选中状态
	if err := c.checkState(imap.ConnStateSelected); err != nil {
		return err // 返回状态检查错误
	}

	// 检查当前会话是否支持移动操作
	session, ok := c.session.(SessionMove)
	if !ok {
		return newClientBugError("移动操作不被支持") // 返回客户端错误信息
	}

	// 创建 MoveWriter 实例
	w := &MoveWriter{conn: c}
	// 调用会话的 Move 方法进行移动操作
	return session.Move(w, numSet, dest)
}

// MoveWriter 用于写入 MOVE 命令的响应。
//
// 服务器必须先调用 WriteCopyData 一次，然后可以调用 WriteExpunge 任意次数。
type MoveWriter struct {
	conn *Conn
}

// WriteCopyData 写入未标记的 COPYUID 响应以处理 MOVE 命令。
// 参数：
//
//	data - 复制数据
//
// 返回：错误信息，如果有的话
func (w *MoveWriter) WriteCopyData(data *imap.CopyData) error {
	return w.conn.writeCopyOK("", data) // 写入复制成功响应
}

// WriteExpunge 写入 EXPUNGE 响应以处理 MOVE 命令。
// 参数：
//
//	seqNum - 邮件的序列号
//
// 返回：错误信息，如果有的话
func (w *MoveWriter) WriteExpunge(seqNum uint32) error {
	return w.conn.writeExpunge(seqNum) // 写入邮件删除响应
}
