package imapclient

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// getSelectOpts 获取选择选项。
func getSelectOpts(options *imap.ListOptions) []string {
	if options == nil {
		return nil
	}

	var l []string
	if options.SelectSubscribed {
		l = append(l, "SUBSCRIBED") // 添加已订阅选项
	}
	if options.SelectRemote {
		l = append(l, "REMOTE") // 添加远程选项
	}
	if options.SelectRecursiveMatch {
		l = append(l, "RECURSIVEMATCH") // 添加递归匹配选项
	}
	if options.SelectSpecialUse {
		l = append(l, "SPECIAL-USE") // 添加特殊用途选项
	}
	return l
}

// getReturnOpts 获取返回选项。
func getReturnOpts(options *imap.ListOptions) []string {
	if options == nil {
		return nil
	}

	var l []string
	if options.ReturnSubscribed {
		l = append(l, "SUBSCRIBED") // 添加已订阅选项
	}
	if options.ReturnChildren {
		l = append(l, "CHILDREN") // 添加子项选项
	}
	if options.ReturnStatus != nil {
		l = append(l, "STATUS") // 添加状态选项
	}
	if options.ReturnSpecialUse {
		l = append(l, "SPECIAL-USE") // 添加特殊用途选项
	}
	return l
}

// List 发送 LIST 命令。
//
// 调用者必须完全消费 ListCommand。一个简单的方法是延迟调用 ListCommand.Close。
//
// nil 的 options 指针等同于零选项值。
//
// 非零的选项值要求支持 IMAP4rev2 或 LIST-EXTENDED 扩展。
func (c *Client) List(ref, pattern string, options *imap.ListOptions) *ListCommand {
	cmd := &ListCommand{
		mailboxes:    make(chan *imap.ListData, 64),
		returnStatus: options != nil && options.ReturnStatus != nil,
	}
	enc := c.beginCommand("LIST", cmd)
	if selectOpts := getSelectOpts(options); len(selectOpts) > 0 {
		enc.SP().List(len(selectOpts), func(i int) {
			enc.Atom(selectOpts[i]) // 添加选择选项
		})
	}
	enc.SP().Mailbox(ref).SP().Mailbox(pattern) // 设置参考和模式
	if returnOpts := getReturnOpts(options); len(returnOpts) > 0 {
		enc.SP().Atom("RETURN").SP().List(len(returnOpts), func(i int) {
			opt := returnOpts[i]
			enc.Atom(opt)
			if opt == "STATUS" {
				returnStatus := statusItems(options.ReturnStatus)
				enc.SP().List(len(returnStatus), func(j int) {
					enc.Atom(returnStatus[j]) // 添加状态项目
				})
			}
		})
	}
	enc.end() // 结束命令
	return cmd
}

// handleList 处理 LIST 响应。
func (c *Client) handleList() error {
	data, err := readList(c.dec) // 读取 LIST 响应
	if err != nil {
		return fmt.Errorf("in LIST: %v", err)
	}

	cmd := c.findPendingCmdFunc(func(cmd command) bool {
		switch cmd := cmd.(type) {
		case *ListCommand:
			return true // TODO: 匹配模式，检查是否已处理
		case *SelectCommand:
			return cmd.mailbox == data.Mailbox && cmd.data.List == nil
		default:
			return false
		}
	})
	switch cmd := cmd.(type) {
	case *ListCommand:
		if cmd.returnStatus {
			if cmd.pendingData != nil {
				cmd.mailboxes <- cmd.pendingData
			}
			cmd.pendingData = data
		} else {
			cmd.mailboxes <- data
		}
	case *SelectCommand:
		cmd.data.List = data
	}

	return nil
}

// ListCommand 是 LIST 命令的结构体。
type ListCommand struct {
	commandBase
	mailboxes chan *imap.ListData // 存储邮箱数据的通道

	returnStatus bool           // 是否返回状态
	pendingData  *imap.ListData // 等待的 LIST 数据
}

// Next 前进到下一个邮箱。
//
// 成功时，返回邮箱 LIST 数据。出错或没有更多邮箱时，返回 nil。
func (cmd *ListCommand) Next() *imap.ListData {
	return <-cmd.mailboxes // 从通道获取下一个邮箱数据
}

// Close 释放命令。
//
// 调用 Close 会解除 IMAP 客户端解码器的阻塞，并让它读取下一个响应。调用 Close 后，Next 将始终返回 nil。
func (cmd *ListCommand) Close() error {
	for cmd.Next() != nil {
		// 忽略
	}
	return cmd.wait() // 等待命令完成
}

// Collect 将邮箱累积到一个列表中。
//
// 这相当于重复调用 Next，然后调用 Close。
func (cmd *ListCommand) Collect() ([]*imap.ListData, error) {
	var l []*imap.ListData
	for {
		data := cmd.Next()
		if data == nil {
			break
		}
		l = append(l, data) // 添加邮箱数据
	}
	return l, cmd.Close() // 返回累积的邮箱数据和关闭命令
}

// readList 读取 LIST 响应。
func readList(dec *imapwire.Decoder) (*imap.ListData, error) {
	var data imap.ListData

	var err error
	data.Attrs, err = internal.ExpectMailboxAttrList(dec) // 读取邮箱属性列表
	if err != nil {
		return nil, fmt.Errorf("in mbx-list-flags: %w", err)
	}

	if !dec.ExpectSP() {
		return nil, dec.Err()
	}

	data.Delim, err = readDelim(dec) // 读取分隔符
	if err != nil {
		return nil, err
	}

	if !dec.ExpectSP() || !dec.ExpectMailbox(&data.Mailbox) {
		return nil, dec.Err()
	}

	if dec.SP() {
		err := dec.ExpectList(func() error {
			var tag string
			if !dec.ExpectAString(&tag) || !dec.ExpectSP() {
				return dec.Err()
			}
			var err error
			switch strings.ToUpper(tag) {
			case "CHILDINFO":
				data.ChildInfo, err = readChildInfoExtendedItem(dec) // 读取子信息扩展项
				if err != nil {
					return fmt.Errorf("in childinfo-extended-item: %v", err)
				}
			case "OLDNAME":
				data.OldName, err = readOldNameExtendedItem(dec) // 读取旧名称扩展项
				if err != nil {
					return fmt.Errorf("in oldname-extended-item: %v", err)
				}
			default:
				if !dec.DiscardValue() {
					return fmt.Errorf("in tagged-ext-val: %v", err)
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("in mbox-list-extended: %v", err)
		}
	}

	return &data, nil
}

// readChildInfoExtendedItem 读取子信息扩展项。
func readChildInfoExtendedItem(dec *imapwire.Decoder) (*imap.ListDataChildInfo, error) {
	var childInfo imap.ListDataChildInfo
	err := dec.ExpectList(func() error {
		var opt string
		if !dec.ExpectAString(&opt) {
			return dec.Err()
		}
		if strings.ToUpper(opt) == "SUBSCRIBED" {
			childInfo.Subscribed = true // 设置已订阅标志
		}
		return nil
	})
	return &childInfo, err
}

// readOldNameExtendedItem 读取旧名称扩展项。
func readOldNameExtendedItem(dec *imapwire.Decoder) (string, error) {
	var name string
	if !dec.ExpectSpecial('(') || !dec.ExpectMailbox(&name) || !dec.ExpectSpecial(')') {
		return "", dec.Err()
	}
	return name, nil
}

// readDelim 读取分隔符。
func readDelim(dec *imapwire.Decoder) (rune, error) {
	var delimStr string
	if dec.Quoted(&delimStr) {
		delim, size := utf8.DecodeRuneInString(delimStr)
		if delim == utf8.RuneError || size != len(delimStr) {
			return 0, fmt.Errorf("mailbox delimiter must be a single rune") // 确保分隔符是单个字符
		}
		return delim, nil
	} else if !dec.ExpectNIL() {
		return 0, dec.Err()
	} else {
		return 0, nil
	}
}
