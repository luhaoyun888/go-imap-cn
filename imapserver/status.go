package imapserver

import (
	"strings"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleStatus 处理 STATUS 命令。
func (c *Conn) handleStatus(dec *imapwire.Decoder) error {
	var mailbox string
	// 检查命令格式，确保包含邮箱名称
	if !dec.ExpectSP() || !dec.ExpectMailbox(&mailbox) || !dec.ExpectSP() {
		return dec.Err() // 返回解码错误
	}

	var options imap.StatusOptions
	recent := false
	// 解析状态项
	err := dec.ExpectList(func() error {
		isRecent, err := readStatusItem(dec, &options) // 读取状态项
		if err != nil {
			return err // 返回读取错误
		} else if isRecent {
			recent = true // 如果状态项是 RECENT，设置标志
		}
		return nil
	})
	if err != nil {
		return err // 返回解析错误
	}

	if !dec.ExpectCRLF() { // 检查命令是否以 CRLF 结束
		return dec.Err() // 返回解码错误
	}

	if err := c.checkState(imap.ConnStateAuthenticated); err != nil { // 检查连接状态是否为已认证
		return err
	}

	data, err := c.session.Status(mailbox, &options) // 调用会话的 Status 方法
	if err != nil {
		return err // 返回状态查询错误
	}

	return c.writeStatus(data, &options, recent) // 写入状态响应
}

// writeStatus 将状态数据写入响应中。
func (c *Conn) writeStatus(data *imap.StatusData, options *imap.StatusOptions, recent bool) error {
	enc := newResponseEncoder(c) // 创建响应编码器
	defer enc.end()              // 确保在函数结束时释放编码器

	// 写入 STATUS 响应的基本信息
	enc.Atom("*").SP().Atom("STATUS").SP().Mailbox(data.Mailbox).SP()
	listEnc := enc.BeginList() // 开始列表
	if options.NumMessages {
		listEnc.Item().Atom("MESSAGES").SP().Number(*data.NumMessages) // 写入消息数量
	}
	if options.UIDNext {
		listEnc.Item().Atom("UIDNEXT").SP().UID(data.UIDNext) // 写入下一个 UID
	}
	if options.UIDValidity {
		listEnc.Item().Atom("UIDVALIDITY").SP().Number(data.UIDValidity) // 写入 UID 有效性
	}
	if options.NumUnseen {
		listEnc.Item().Atom("UNSEEN").SP().Number(*data.NumUnseen) // 写入未读消息数量
	}
	if options.NumDeleted {
		listEnc.Item().Atom("DELETED").SP().Number(*data.NumDeleted) // 写入已删除消息数量
	}
	if options.Size {
		listEnc.Item().Atom("SIZE").SP().Number64(*data.Size) // 写入邮箱大小
	}
	if options.AppendLimit {
		listEnc.Item().Atom("APPENDLIMIT").SP()
		if data.AppendLimit != nil {
			enc.Number(*data.AppendLimit) // 写入追加限制
		} else {
			enc.NIL() // 写入 NIL
		}
	}
	if options.DeletedStorage {
		listEnc.Item().Atom("DELETED-STORAGE").SP().Number64(*data.DeletedStorage) // 写入已删除存储
	}
	if recent {
		listEnc.Item().Atom("RECENT").SP().Number(0) // 写入 RECENT 标志
	}
	listEnc.End() // 结束列表

	return enc.CRLF() // 返回 CRLF 表示响应结束
}

// readStatusItem 读取状态项并更新选项。
func readStatusItem(dec *imapwire.Decoder, options *imap.StatusOptions) (isRecent bool, err error) {
	var name string
	if !dec.ExpectAtom(&name) { // 读取状态项名称
		return false, dec.Err() // 返回解码错误
	}
	switch strings.ToUpper(name) { // 将名称转为大写以进行匹配
	case "MESSAGES":
		options.NumMessages = true // 设置消息数量标志
	case "UIDNEXT":
		options.UIDNext = true // 设置下一个 UID 标志
	case "UIDVALIDITY":
		options.UIDValidity = true // 设置 UID 有效性标志
	case "UNSEEN":
		options.NumUnseen = true // 设置未读消息数量标志
	case "DELETED":
		options.NumDeleted = true // 设置已删除消息数量标志
	case "SIZE":
		options.Size = true // 设置邮箱大小标志
	case "APPENDLIMIT":
		options.AppendLimit = true // 设置追加限制标志
	case "DELETED-STORAGE":
		options.DeletedStorage = true // 设置已删除存储标志
	case "RECENT":
		isRecent = true // 设置 RECENT 标志
	default:
		return false, &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "未知的 STATUS 数据项", // 返回未知状态项错误
		}
	}
	return isRecent, nil // 返回读取结果
}
