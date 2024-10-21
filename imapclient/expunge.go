package imapclient

import (
	"github.com/luhaoyun888/go-imap-cn"
)

// Expunge 发送 EXPUNGE 命令。
func (c *Client) Expunge() *ExpungeCommand {
	cmd := &ExpungeCommand{seqNums: make(chan uint32, 128)} // 创建一个 EXPUNGE 命令
	c.beginCommand("EXPUNGE", cmd).end()                    // 开始命令
	return cmd
}

// UIDExpunge 发送 UID EXPUNGE 命令。
//
// 此命令要求支持 IMAP4rev2 或 UIDPLUS 扩展。
func (c *Client) UIDExpunge(uids imap.UIDSet) *ExpungeCommand {
	cmd := &ExpungeCommand{seqNums: make(chan uint32, 128)} // 创建一个 UID EXPUNGE 命令
	enc := c.beginCommand("UID EXPUNGE", cmd)               // 开始命令
	enc.SP().NumSet(uids)                                   // 设置 UID
	enc.end()                                               // 结束命令
	return cmd
}

// handleExpunge 处理 EXPUNGE 响应。
func (c *Client) handleExpunge(seqNum uint32) error {
	c.mutex.Lock() // 锁定以保护状态
	if c.state == imap.ConnStateSelected && c.mailbox.NumMessages > 0 {
		c.mailbox = c.mailbox.copy() // 复制邮箱状态
		c.mailbox.NumMessages--      // 减少邮件数量
	}
	c.mutex.Unlock() // 解锁

	cmd := findPendingCmdByType[*ExpungeCommand](c) // 查找待处理的命令
	if cmd != nil {
		cmd.seqNums <- seqNum // 将序列号发送到命令
	} else if handler := c.options.unilateralDataHandler().Expunge; handler != nil {
		handler(seqNum) // 调用处理程序
	}

	return nil
}

// ExpungeCommand 是一个 EXPUNGE 命令。
//
// 调用者必须完全消耗 ExpungeCommand。一个简单的方法是
// 延迟调用 FetchCommand.Close。
type ExpungeCommand struct {
	commandBase
	seqNums chan uint32 // 存储序列号的通道
}

// Next 前进到下一个被删除的邮件序列号。
//
// 成功时返回邮件序列号。出错或没有更多邮件时返回 0。
// 要检查错误值，请使用 Close。
func (cmd *ExpungeCommand) Next() uint32 {
	return <-cmd.seqNums // 从通道中接收序列号
}

// Close 释放命令。
//
// 调用 Close 会解锁 IMAP 客户端解码器，并让它读取下一个
// 响应。Close 后 Next 始终返回 nil。
func (cmd *ExpungeCommand) Close() error {
	for cmd.Next() != 0 {
		// 忽略
	}
	return cmd.wait() // 等待命令完成
}

// Collect 将被删除的序列号累积到列表中。
//
// 这等效于重复调用 Next 然后 Close。
func (cmd *ExpungeCommand) Collect() ([]uint32, error) {
	var l []uint32 // 存储序列号的列表
	for {
		seqNum := cmd.Next() // 获取下一个序列号
		if seqNum == 0 {
			break // 没有更多序列号
		}
		l = append(l, seqNum) // 将序列号添加到列表
	}
	return l, cmd.Close() // 返回列表和关闭命令
}
