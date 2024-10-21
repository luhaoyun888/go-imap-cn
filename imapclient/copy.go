package imapclient

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// Copy 发送 COPY 命令。
// 参数：
//
//	numSet - 要复制的邮件编号集合。
//	mailbox - 目标邮箱的名称。
//
// 返回值：
//
//	*CopyCommand - 复制命令的实例，用于后续操作。
func (c *Client) Copy(numSet imap.NumSet, mailbox string) *CopyCommand {
	cmd := &CopyCommand{}                                                       // 创建一个新的 CopyCommand 实例
	enc := c.beginCommand(uidCmdName("COPY", imapwire.NumSetKind(numSet)), cmd) // 开始 COPY 命令
	enc.SP().NumSet(numSet).SP().Mailbox(mailbox)                               // 设置命令参数
	enc.end()                                                                   // 结束命令
	return cmd                                                                  // 返回 COPY 命令实例
}

// CopyCommand 是一个 COPY 命令的结构体。
type CopyCommand struct {
	commandBase               // 基础命令结构体
	data        imap.CopyData // 存储复制操作的相关数据
}

// Wait 等待 COPY 命令的响应。
// 返回值：
//
//	*imap.CopyData - 复制数据的指针。
//	error - 在等待过程中发生的错误。
func (cmd *CopyCommand) Wait() (*imap.CopyData, error) {
	return &cmd.data, cmd.wait() // 返回复制数据和错误
}

// readRespCodeCopyUID 读取 COPYUID 响应中的响应代码。
// 参数：
//
//	dec - 用于解码 IMAP 响应的解码器。
//
// 返回值：
//
//	uidValidity - UID 有效性。
//	srcUIDs - 源 UID 集合。
//	dstUIDs - 目标 UID 集合。
//	error - 处理过程中的错误。
func readRespCodeCopyUID(dec *imapwire.Decoder) (uidValidity uint32, srcUIDs, dstUIDs imap.UIDSet, err error) {
	if !dec.ExpectNumber(&uidValidity) || !dec.ExpectSP() || !dec.ExpectUIDSet(&srcUIDs) || !dec.ExpectSP() || !dec.ExpectUIDSet(&dstUIDs) {
		return 0, nil, nil, dec.Err() // 解析失败，返回错误
	}
	if srcUIDs.Dynamic() || dstUIDs.Dynamic() {
		return 0, nil, nil, fmt.Errorf("imapclient: 服务器在 COPYUID 响应中返回了动态编号集") // 报告错误
	}
	return uidValidity, srcUIDs, dstUIDs, nil // 返回解析结果
}
