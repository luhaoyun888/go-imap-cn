package imapclient

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
)

// Select 发送 SELECT 或 EXAMINE 命令。
//
// nil 的选项指针等同于零选项值。
func (c *Client) Select(mailbox string, options *imap.SelectOptions) *SelectCommand {
	cmdName := "SELECT"                     // 默认命令为 SELECT
	if options != nil && options.ReadOnly { // 如果选项为只读，则使用 EXAMINE 命令
		cmdName = "EXAMINE"
	}

	cmd := &SelectCommand{mailbox: mailbox}  // 创建选择命令
	enc := c.beginCommand(cmdName, cmd)      // 开始命令编码
	enc.SP().Mailbox(mailbox)                // 添加邮箱参数
	if options != nil && options.CondStore { // 如果启用条件存储
		enc.SP().Special('(').Atom("CONDSTORE").Special(')') // 添加条件存储标志
	}
	enc.end()  // 结束命令
	return cmd // 返回选择命令
}

// Unselect 发送 UNSELECT 命令。
//
// 此命令要求支持 IMAP4rev2 或 UNSELECT 扩展。
func (c *Client) Unselect() *Command {
	cmd := &unselectCommand{}             // 创建 UNSELECT 命令
	c.beginCommand("UNSELECT", cmd).end() // 开始并结束命令
	return &cmd.Command                   // 返回命令
}

// UnselectAndExpunge 发送 CLOSE 命令。
//
// CLOSE 隐式执行静默 EXPUNGE 命令。
func (c *Client) UnselectAndExpunge() *Command {
	cmd := &unselectCommand{}          // 创建 UNSELECT 命令
	c.beginCommand("CLOSE", cmd).end() // 开始并结束命令
	return &cmd.Command                // 返回命令
}

func (c *Client) handleFlags() error {
	flags, err := internal.ExpectFlagList(c.dec) // 读取标志列表
	if err != nil {
		return err // 如果有错误，返回错误
	}

	c.mutex.Lock()                         // 锁定以避免并发问题
	if c.state == imap.ConnStateSelected { // 如果状态为选中
		c.mailbox = c.mailbox.copy()     // 复制当前邮箱
		c.mailbox.PermanentFlags = flags // 更新永久标志
	}
	c.mutex.Unlock() // 解锁

	cmd := findPendingCmdByType[*SelectCommand](c) // 查找待处理的选择命令
	if cmd != nil {
		cmd.data.Flags = flags // 更新命令的数据标志
	} else if handler := c.options.unilateralDataHandler().Mailbox; handler != nil {
		handler(&UnilateralDataMailbox{Flags: flags}) // 调用处理程序
	}

	return nil // 返回成功
}

func (c *Client) handleExists(num uint32) error {
	cmd := findPendingCmdByType[*SelectCommand](c) // 查找待处理的选择命令
	if cmd != nil {
		cmd.data.NumMessages = num // 更新命令的数据消息数
	} else {
		c.mutex.Lock()                         // 锁定以避免并发问题
		if c.state == imap.ConnStateSelected { // 如果状态为选中
			c.mailbox = c.mailbox.copy() // 复制当前邮箱
			c.mailbox.NumMessages = num  // 更新消息数量
		}
		c.mutex.Unlock() // 解锁

		if handler := c.options.unilateralDataHandler().Mailbox; handler != nil {
			handler(&UnilateralDataMailbox{NumMessages: &num}) // 调用处理程序
		}
	}
	return nil // 返回成功
}

// SelectCommand 是 SELECT 命令。
type SelectCommand struct {
	commandBase
	mailbox string          // 邮箱名称
	data    imap.SelectData // 选择数据
}

func (cmd *SelectCommand) Wait() (*imap.SelectData, error) {
	return &cmd.data, cmd.wait() // 等待命令完成并返回选择数据
}

type unselectCommand struct {
	Command // UNSELECT 命令
}
