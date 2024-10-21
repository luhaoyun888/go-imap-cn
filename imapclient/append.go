package imapclient

import (
	"io"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
)

// Append 发送 APPEND 命令。
//
// 调用者必须调用 AppendCommand.Close 方法。
//
// options 是可选的。
func (c *Client) Append(mailbox string, size int64, options *imap.AppendOptions) *AppendCommand {
	cmd := &AppendCommand{}
	cmd.enc = c.beginCommand("APPEND", cmd) // 开始 APPEND 命令
	cmd.enc.SP().Mailbox(mailbox).SP()      // 设置邮箱名称
	if options != nil && len(options.Flags) > 0 {
		cmd.enc.List(len(options.Flags), func(i int) {
			cmd.enc.Flag(options.Flags[i]) // 添加标志
		}).SP()
	}
	if options != nil && !options.Time.IsZero() {
		cmd.enc.String(options.Time.Format(internal.DateTimeLayout)).SP() // 设置时间
	}
	// TODO: literal8 for BINARY
	// TODO: UTF8 data ext for UTF8=ACCEPT, with literal8
	cmd.wc = cmd.enc.Literal(size) // 设置字面量大小
	return cmd
}

// AppendCommand 是一个 APPEND 命令。
//
// 调用者必须写入消息内容，然后调用 Close 方法。
type AppendCommand struct {
	commandBase
	enc  *commandEncoder // 命令编码器
	wc   io.WriteCloser  // 写入关闭器
	data imap.AppendData // APPEND 数据
}

// Write 将字节写入命令。
func (cmd *AppendCommand) Write(b []byte) (int, error) {
	return cmd.wc.Write(b)
}

// Close 关闭命令，等待服务器响应。
func (cmd *AppendCommand) Close() error {
	err := cmd.wc.Close() // 关闭写入器
	if cmd.enc != nil {
		cmd.enc.end() // 结束命令
		cmd.enc = nil
	}
	return err
}

// Wait 等待 APPEND 命令的响应，并返回数据。
func (cmd *AppendCommand) Wait() (*imap.AppendData, error) {
	return &cmd.data, cmd.wait()
}
