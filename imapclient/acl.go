package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// MyRights 发送 MYRIGHTS 命令。
//
// 此命令需要支持 ACL 扩展。
func (c *Client) MyRights(mailbox string) *MyRightsCommand {
	cmd := &MyRightsCommand{}
	enc := c.beginCommand("MYRIGHTS", cmd)
	enc.SP().Mailbox(mailbox) // 设置邮箱
	enc.end()
	return cmd
}

// SetACL 发送 SETACL 命令。
//
// 此命令需要支持 ACL 扩展。
func (c *Client) SetACL(mailbox string, ri imap.RightsIdentifier, rm imap.RightModification, rs imap.RightSet) *SetACLCommand {
	cmd := &SetACLCommand{}
	enc := c.beginCommand("SETACL", cmd)
	enc.SP().Mailbox(mailbox).SP().String(string(ri)).SP() // 设置邮箱和权限标识符
	enc.String(internal.FormatRights(rm, rs))              // 格式化并设置权限
	enc.end()
	return cmd
}

// SetACLCommand 是一个 SETACL 命令。
type SetACLCommand struct {
	commandBase
}

// Wait 等待 SETACL 命令的响应。
func (cmd *SetACLCommand) Wait() error {
	return cmd.wait()
}

// GetACL 发送 GETACL 命令。
//
// 此命令需要支持 ACL 扩展。
func (c *Client) GetACL(mailbox string) *GetACLCommand {
	cmd := &GetACLCommand{}
	enc := c.beginCommand("GETACL", cmd)
	enc.SP().Mailbox(mailbox) // 设置邮箱
	enc.end()
	return cmd
}

// GetACLCommand 是一个 GETACL 命令。
type GetACLCommand struct {
	commandBase
	data GetACLData
}

// Wait 等待 GETACL 命令的响应，并返回数据。
func (cmd *GetACLCommand) Wait() (*GetACLData, error) {
	return &cmd.data, cmd.wait()
}

// handleMyRights 处理 MYRIGHTS 响应。
func (c *Client) handleMyRights() error {
	data, err := readMyRights(c.dec)
	if err != nil {
		return fmt.Errorf("在 myrights 响应中: %v", err)
	}
	if cmd := findPendingCmdByType[*MyRightsCommand](c); cmd != nil {
		cmd.data = *data
	}
	return nil
}

// handleGetACL 处理 GETACL 响应。
func (c *Client) handleGetACL() error {
	data, err := readGetACL(c.dec)
	if err != nil {
		return fmt.Errorf("在 getacl 响应中: %v", err)
	}
	if cmd := findPendingCmdByType[*GetACLCommand](c); cmd != nil {
		cmd.data = *data
	}
	return nil
}

// MyRightsCommand 是一个 MYRIGHTS 命令。
type MyRightsCommand struct {
	commandBase
	data MyRightsData
}

// Wait 等待 MYRIGHTS 命令的响应，并返回数据。
func (cmd *MyRightsCommand) Wait() (*MyRightsData, error) {
	return &cmd.data, cmd.wait()
}

// MyRightsData 是 MYRIGHTS 命令返回的数据。
type MyRightsData struct {
	Mailbox string        // 邮箱名称
	Rights  imap.RightSet // 权限集
}

// readMyRights 从解码器读取 MYRIGHTS 数据。
func readMyRights(dec *imapwire.Decoder) (*MyRightsData, error) {
	var (
		rights string
		data   MyRightsData
	)
	if !dec.ExpectMailbox(&data.Mailbox) || !dec.ExpectSP() || !dec.ExpectAString(&rights) {
		return nil, dec.Err()
	}

	data.Rights = imap.RightSet(rights)
	return &data, nil
}

// GetACLData 是 GETACL 命令返回的数据。
type GetACLData struct {
	Mailbox string                                  // 邮箱名称
	Rights  map[imap.RightsIdentifier]imap.RightSet // 权限集合
}

// readGetACL 从解码器读取 GETACL 数据。
func readGetACL(dec *imapwire.Decoder) (*GetACLData, error) {
	data := &GetACLData{Rights: make(map[imap.RightsIdentifier]imap.RightSet)}

	if !dec.ExpectMailbox(&data.Mailbox) {
		return nil, dec.Err()
	}

	for dec.SP() {
		var rsStr, riStr string
		if !dec.ExpectAString(&riStr) || !dec.ExpectSP() || !dec.ExpectAString(&rsStr) {
			return nil, dec.Err()
		}

		data.Rights[imap.RightsIdentifier(riStr)] = imap.RightSet(rsStr)
	}

	return data, nil
}
