package imapclient

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
)

// Enable 发送 ENABLE 命令。
// 该命令需要支持 IMAP4rev2 或 ENABLE 扩展。
// 参数：
//
//	caps - 要启用的能力列表。
func (c *Client) Enable(caps ...imap.Cap) *EnableCommand {
	// 启用扩展可能会更改 IMAP 语法，因此只允许支持的扩展
	for _, name := range caps {
		switch name {
		case imap.CapIMAP4rev2, imap.CapUTF8Accept, imap.CapMetadata, imap.CapMetadataServer:
			// 支持的扩展，继续
		default:
			done := make(chan error)                                              // 创建完成信道
			close(done)                                                           // 关闭信道
			err := fmt.Errorf("imapclient: 无法启用 %q: 不支持", name)                   // 返回不支持错误
			return &EnableCommand{commandBase: commandBase{done: done, err: err}} // 返回 ENABLE 命令实例
		}
	}

	cmd := &EnableCommand{}              // 创建 ENABLE 命令实例
	enc := c.beginCommand("ENABLE", cmd) // 开始 ENABLE 命令
	for _, c := range caps {
		enc.SP().Atom(string(c)) // 添加要启用的能力
	}
	enc.end()  // 结束命令
	return cmd // 返回 ENABLE 命令实例
}

// handleEnabled 处理 ENABLE 命令的响应。
// 返回值：
//
//	error - 处理过程中的错误。
func (c *Client) handleEnabled() error {
	caps, err := readCapabilities(c.dec) // 读取能力
	if err != nil {
		return err // 返回错误
	}

	c.mutex.Lock() // 锁定互斥体
	for name := range caps {
		c.enabled[name] = struct{}{} // 将启用的能力存入
	}
	c.mutex.Unlock() // 解锁互斥体

	if cmd := findPendingCmdByType[*EnableCommand](c); cmd != nil { // 查找待处理的 ENABLE 命令
		cmd.data.Caps = caps // 更新 ENABLE 命令的数据
	}

	return nil // 返回 nil，表示成功
}

// EnableCommand 是 ENABLE 命令的结构体。
type EnableCommand struct {
	commandBase            // 基本命令结构体
	data        EnableData // 启用命令返回的数据
}

// Wait 等待 ENABLE 命令的完成并返回数据。
// 返回值：
//
//	*EnableData - 启用命令返回的数据。
//	error - 等待过程中产生的错误。
func (cmd *EnableCommand) Wait() (*EnableData, error) {
	return &cmd.data, cmd.wait() // 返回数据和等待结果
}

// EnableData 是 ENABLE 命令返回的数据结构体。
type EnableData struct {
	// 成功启用的能力
	Caps imap.CapSet // 启用的能力集
}
