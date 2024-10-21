package imapclient

import (
	"github.com/luhaoyun888/go-imap-cn"
)

// Create 发送 CREATE 命令，用于创建新的邮箱。
// 参数：
//
//	mailbox - 要创建的邮箱名称。
//	options - 创建选项，指向 imap.CreateOptions 结构体。
//	          nil 值的选项指针等同于零值选项。
//
// 返回值：
//
//	*Command - CREATE 命令的实例，用于后续操作。
func (c *Client) Create(mailbox string, options *imap.CreateOptions) *Command {
	cmd := &Command{}                    // 创建一个新的 Command 实例
	enc := c.beginCommand("CREATE", cmd) // 开始 CREATE 命令
	enc.SP().Mailbox(mailbox)            // 设置邮箱名称

	if options != nil && len(options.SpecialUse) > 0 { // 检查是否有特殊用途选项
		enc.SP().Special('(').Atom("USE").SP().List(len(options.SpecialUse), func(i int) { // 开始特殊用途列表
			enc.MailboxAttr(options.SpecialUse[i]) // 添加每个特殊用途
		}).Special(')') // 结束特殊用途列表
	}
	enc.end()  // 结束命令
	return cmd // 返回 CREATE 命令实例
}
