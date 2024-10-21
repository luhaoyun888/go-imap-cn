package imapclient

import (
	"fmt"
	"strings"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// statusItems 根据状态选项返回需要的状态项列表
func statusItems(options *imap.StatusOptions) []string {
	m := map[string]bool{
		"MESSAGES":        options.NumMessages,    // 消息数量
		"UIDNEXT":         options.UIDNext,        // 下一个 UID
		"UIDVALIDITY":     options.UIDValidity,    // UID 有效性
		"UNSEEN":          options.NumUnseen,      // 未读消息数量
		"DELETED":         options.NumDeleted,     // 删除消息数量
		"SIZE":            options.Size,           // 邮箱大小
		"APPENDLIMIT":     options.AppendLimit,    // 附加限制
		"DELETED-STORAGE": options.DeletedStorage, // 删除存储
		"HIGHESTMODSEQ":   options.HighestModSeq,  // 最高修改序列号
	}

	var l []string
	for k, req := range m {
		if req {
			l = append(l, k) // 添加请求的状态项
		}
	}
	return l
}

// Status 发送一个 STATUS 命令。
//
// 一个 nil 的选项指针相当于零选项值。
func (c *Client) Status(mailbox string, options *imap.StatusOptions) *StatusCommand {
	if options == nil {
		options = new(imap.StatusOptions) // 如果选项为 nil，则创建新选项
	}

	cmd := &StatusCommand{mailbox: mailbox}
	enc := c.beginCommand("STATUS", cmd)
	enc.SP().Mailbox(mailbox).SP() // 添加邮箱名称
	items := statusItems(options)  // 获取状态项列表
	enc.List(len(items), func(i int) {
		enc.Atom(items[i]) // 添加状态项
	})
	enc.end()
	return cmd
}

func (c *Client) handleStatus() error {
	data, err := readStatus(c.dec) // 读取状态数据
	if err != nil {
		return fmt.Errorf("在状态中: %v", err) // 返回错误信息
	}

	cmd := c.findPendingCmdFunc(func(cmd command) bool {
		switch cmd := cmd.(type) {
		case *StatusCommand:
			return cmd.mailbox == data.Mailbox // 匹配邮箱名称
		case *ListCommand:
			return cmd.returnStatus && cmd.pendingData != nil && cmd.pendingData.Mailbox == data.Mailbox
		default:
			return false
		}
	})
	switch cmd := cmd.(type) {
	case *StatusCommand:
		cmd.data = *data // 将状态数据赋值给命令
	case *ListCommand:
		cmd.pendingData.Status = data
		cmd.mailboxes <- cmd.pendingData
		cmd.pendingData = nil
	}

	return nil
}

// StatusCommand 是一个 STATUS 命令。
type StatusCommand struct {
	commandBase
	mailbox string          // 邮箱名称
	data    imap.StatusData // 状态数据
}

// Wait 等待状态命令的完成，并返回状态数据
func (cmd *StatusCommand) Wait() (*imap.StatusData, error) {
	return &cmd.data, cmd.wait() // 返回状态数据和等待结果
}

// readStatus 读取状态数据
func readStatus(dec *imapwire.Decoder) (*imap.StatusData, error) {
	var data imap.StatusData

	if !dec.ExpectMailbox(&data.Mailbox) || !dec.ExpectSP() {
		return nil, dec.Err() // 返回错误
	}

	err := dec.ExpectList(func() error {
		if err := readStatusAttVal(dec, &data); err != nil {
			return fmt.Errorf("在状态属性值中: %v", dec.Err())
		}
		return nil
	})
	return &data, err
}

// readStatusAttVal 读取状态属性值
func readStatusAttVal(dec *imapwire.Decoder, data *imap.StatusData) error {
	var name string
	if !dec.ExpectAtom(&name) || !dec.ExpectSP() {
		return dec.Err() // 返回错误
	}

	var ok bool
	switch strings.ToUpper(name) {
	case "MESSAGES":
		var num uint32
		ok = dec.ExpectNumber(&num)
		data.NumMessages = &num // 设置消息数量
	case "UIDNEXT":
		var uidNext imap.UID
		ok = dec.ExpectUID(&uidNext)
		data.UIDNext = uidNext // 设置下一个 UID
	case "UIDVALIDITY":
		ok = dec.ExpectNumber(&data.UIDValidity) // 设置 UID 有效性
	case "UNSEEN":
		var num uint32
		ok = dec.ExpectNumber(&num)
		data.NumUnseen = &num // 设置未读消息数量
	case "DELETED":
		var num uint32
		ok = dec.ExpectNumber(&num)
		data.NumDeleted = &num // 设置删除消息数量
	case "SIZE":
		var size int64
		ok = dec.ExpectNumber64(&size)
		data.Size = &size // 设置邮箱大小
	case "APPENDLIMIT":
		var num uint32
		if dec.Number(&num) {
			ok = true
		} else {
			ok = dec.ExpectNIL() // 期望为 NIL
			num = ^uint32(0)     // 设置为最大值
		}
		data.AppendLimit = &num // 设置附加限制
	case "DELETED-STORAGE":
		var storage int64
		ok = dec.ExpectNumber64(&storage)
		data.DeletedStorage = &storage // 设置删除存储
	case "HIGHESTMODSEQ":
		ok = dec.ExpectModSeq(&data.HighestModSeq) // 设置最高修改序列号
	default:
		if !dec.DiscardValue() {
			return dec.Err() // 返回错误
		}
	}
	if !ok {
		return dec.Err() // 返回错误
	}
	return nil
}
