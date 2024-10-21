package imapserver

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleSelect 处理 SELECT 命令，选择一个邮箱。
// tag: 请求的标记，用于响应。
// dec: 解码器，用于解析输入数据。
// readOnly: 指示选择的邮箱是否为只读模式。
func (c *Conn) handleSelect(tag string, dec *imapwire.Decoder, readOnly bool) error {
	var mailbox string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&mailbox) || !dec.ExpectCRLF() {
		return dec.Err()
	}

	// 检查连接状态是否为已认证状态。
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}

	// 如果当前状态是已选择状态，则先取消选择。
	if c.state == imap.ConnStateSelected {
		if err := c.session.Unselect(); err != nil {
			return err
		}
		c.state = imap.ConnStateAuthenticated
		err := c.writeStatusResp("", &imap.StatusResponse{
			Type: imap.StatusResponseTypeOK,
			Code: "CLOSED",
			Text: "上一个邮箱现已关闭",
		})
		if err != nil {
			return err
		}
	}

	// 设置选择选项。
	options := imap.SelectOptions{ReadOnly: readOnly}
	data, err := c.session.Select(mailbox, &options)
	if err != nil {
		return err
	}

	// 写入邮箱中的消息数量。
	if err := c.writeExists(data.NumMessages); err != nil {
		return err
	}
	// 如果不支持 IMAP4rev2，写入过时的 RECENT。
	if !c.enabled.Has(imap.CapIMAP4rev2) {
		if err := c.writeObsoleteRecent(); err != nil {
			return err
		}
	}
	// 写入 UID 有效性。
	if err := c.writeUIDValidity(data.UIDValidity); err != nil {
		return err
	}
	// 写入下一个 UID。
	if err := c.writeUIDNext(data.UIDNext); err != nil {
		return err
	}
	// 写入标志。
	if err := c.writeFlags(data.Flags); err != nil {
		return err
	}
	// 写入永久标志。
	if err := c.writePermanentFlags(data.PermanentFlags); err != nil {
		return err
	}
	// 如果有列表数据，写入列表。
	if data.List != nil {
		if err := c.writeList(data.List); err != nil {
			return err
		}
	}

	c.state = imap.ConnStateSelected
	// TODO: 在只读模式下禁止写命令

	var (
		cmdName string
		code    imap.ResponseCode
	)
	if readOnly {
		cmdName = "EXAMINE"
		code = "READ-ONLY"
	} else {
		cmdName = "SELECT"
		code = "READ-WRITE"
	}
	return c.writeStatusResp(tag, &imap.StatusResponse{
		Type: imap.StatusResponseTypeOK,
		Code: code,
		Text: fmt.Sprintf("%v 完成", cmdName),
	})
}

// handleUnselect 处理 UNSELECT 命令，取消当前选择的邮箱。
// dec: 解码器，用于解析输入数据。
// expunge: 指示是否在取消选择时清除已删除邮件。
func (c *Conn) handleUnselect(dec *imapwire.Decoder, expunge bool) error {
	if !dec.ExpectCRLF() {
		return dec.Err()
	}

	// 检查连接状态是否为已选择状态。
	if err := c.checkState(imap.ConnStateSelected); err != nil {
		return err
	}

	// 如果需要，清除已删除邮件。
	if expunge {
		w := &ExpungeWriter{}
		if err := c.session.Expunge(w, nil); err != nil {
			return err
		}
	}

	// 取消选择当前邮箱。
	if err := c.session.Unselect(); err != nil {
		return err
	}

	c.state = imap.ConnStateAuthenticated
	return nil
}

// writeExists 写入邮箱中存在的消息数量。
// numMessages: 邮箱中消息的数量。
func (c *Conn) writeExists(numMessages uint32) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	return enc.Atom("*").SP().Number(numMessages).SP().Atom("EXISTS").CRLF()
}

// writeObsoleteRecent 写入过时的 RECENT 响应，表示没有最近的邮件。
// 返回值：无。
func (c *Conn) writeObsoleteRecent() error {
	enc := newResponseEncoder(c)
	defer enc.end()
	return enc.Atom("*").SP().Number(0).SP().Atom("RECENT").CRLF()
}

// writeUIDValidity 写入 UID 有效性。
// uidValidity: UID 有效性值。
func (c *Conn) writeUIDValidity(uidValidity uint32) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("OK").SP()
	enc.Special('[').Atom("UIDVALIDITY").SP().Number(uidValidity).Special(']')
	enc.SP().Text("UIDs 有效")
	return enc.CRLF()
}

// writeUIDNext 写入下一个 UID。
// uidNext: 预测的下一个 UID。
func (c *Conn) writeUIDNext(uidNext imap.UID) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("OK").SP()
	enc.Special('[').Atom("UIDNEXT").SP().UID(uidNext).Special(']')
	enc.SP().Text("预测的下一个 UID")
	return enc.CRLF()
}

// writeFlags 写入邮箱中使用的标志。
// flags: 邮箱中使用的标志列表。
func (c *Conn) writeFlags(flags []imap.Flag) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("FLAGS").SP().List(len(flags), func(i int) {
		enc.Flag(flags[i])
	})
	return enc.CRLF()
}

// writePermanentFlags 写入邮箱中的永久标志。
// flags: 邮箱中的永久标志列表。
func (c *Conn) writePermanentFlags(flags []imap.Flag) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("OK").SP()
	enc.Special('[').Atom("PERMANENTFLAGS").SP().List(len(flags), func(i int) {
		enc.Flag(flags[i])
	}).Special(']')
	enc.SP().Text("永久标志")
	return enc.CRLF()
}
