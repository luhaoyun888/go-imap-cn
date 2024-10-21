package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// GetQuota 发送 GETQUOTA 命令。
//
// 此命令要求支持 QUOTA 扩展。
func (c *Client) GetQuota(root string) *GetQuotaCommand {
	cmd := &GetQuotaCommand{root: root}
	enc := c.beginCommand("GETQUOTA", cmd)
	enc.SP().String(root) // 添加根命名空间
	enc.end()
	return cmd
}

// GetQuotaRoot 发送 GETQUOTAROOT 命令。
//
// 此命令要求支持 QUOTA 扩展。
func (c *Client) GetQuotaRoot(mailbox string) *GetQuotaRootCommand {
	cmd := &GetQuotaRootCommand{mailbox: mailbox}
	enc := c.beginCommand("GETQUOTAROOT", cmd)
	enc.SP().Mailbox(mailbox) // 添加邮箱
	enc.end()
	return cmd
}

// SetQuota 发送 SETQUOTA 命令。
//
// 此命令要求支持 SETQUOTA 扩展。
func (c *Client) SetQuota(root string, limits map[imap.QuotaResourceType]int64) *Command {
	// TODO: 考虑返回 QUOTA 响应数据？
	cmd := &Command{}
	enc := c.beginCommand("SETQUOTA", cmd)
	enc.SP().String(root).SP().Special('(') // 添加根命名空间和限制值
	i := 0
	for typ, limit := range limits {
		if i > 0 {
			enc.SP()
		}
		enc.Atom(string(typ)).SP().Number64(limit) // 添加每个资源的使用限制
		i++
	}
	enc.Special(')') // 结束限制值的列表
	enc.end()
	return cmd
}

// handleQuota 处理 QUOTA 响应。
func (c *Client) handleQuota() error {
	data, err := readQuotaResponse(c.dec) // 读取 QUOTA 响应
	if err != nil {
		return fmt.Errorf("in quota-response: %v", err)
	}

	cmd := c.findPendingCmdFunc(func(cmd command) bool {
		switch cmd := cmd.(type) {
		case *GetQuotaCommand:
			return cmd.root == data.Root // 匹配根命名空间
		case *GetQuotaRootCommand:
			for _, root := range cmd.roots {
				if root == data.Root {
					return true
				}
			}
			return false
		default:
			return false
		}
	})
	switch cmd := cmd.(type) {
	case *GetQuotaCommand:
		cmd.data = data // 设置响应数据
	case *GetQuotaRootCommand:
		cmd.data = append(cmd.data, *data) // 追加响应数据
	}
	return nil
}

// handleQuotaRoot 处理 QUOTAROOT 响应。
func (c *Client) handleQuotaRoot() error {
	mailbox, roots, err := readQuotaRoot(c.dec) // 读取 QUOTAROOT 响应
	if err != nil {
		return fmt.Errorf("in quotaroot-response: %v", err)
	}

	cmd := c.findPendingCmdFunc(func(anyCmd command) bool {
		cmd, ok := anyCmd.(*GetQuotaRootCommand)
		if !ok {
			return false
		}
		return cmd.mailbox == mailbox // 匹配邮箱
	})
	if cmd != nil {
		cmd := cmd.(*GetQuotaRootCommand)
		cmd.roots = roots // 设置根命名空间
	}
	return nil
}

// GetQuotaCommand 是 GETQUOTA 命令的结构体。
type GetQuotaCommand struct {
	commandBase
	root string     // 根命名空间
	data *QuotaData // 响应数据
}

// Wait 等待命令完成，并返回 QUOTA 数据。
func (cmd *GetQuotaCommand) Wait() (*QuotaData, error) {
	if err := cmd.wait(); err != nil {
		return nil, err
	}
	return cmd.data, nil
}

// GetQuotaRootCommand 是 GETQUOTAROOT 命令的结构体。
type GetQuotaRootCommand struct {
	commandBase
	mailbox string      // 邮箱名称
	roots   []string    // 根命名空间列表
	data    []QuotaData // 响应数据列表
}

// Wait 等待命令完成，并返回 QUOTA 数据列表。
func (cmd *GetQuotaRootCommand) Wait() ([]QuotaData, error) {
	if err := cmd.wait(); err != nil {
		return nil, err
	}
	return cmd.data, nil
}

// QuotaData 是 QUOTA 响应返回的数据。
type QuotaData struct {
	Root      string                                       // 根命名空间
	Resources map[imap.QuotaResourceType]QuotaResourceData // 资源数据
}

// QuotaResourceData 包含配额资源的使用情况和限制。
type QuotaResourceData struct {
	Usage int64 // 使用量
	Limit int64 // 限制量
}

// readQuotaResponse 读取 QUOTA 响应。
func readQuotaResponse(dec *imapwire.Decoder) (*QuotaData, error) {
	var data QuotaData
	if !dec.ExpectAString(&data.Root) || !dec.ExpectSP() {
		return nil, dec.Err()
	}
	data.Resources = make(map[imap.QuotaResourceType]QuotaResourceData)
	err := dec.ExpectList(func() error {
		var (
			name    string
			resData QuotaResourceData
		)
		if !dec.ExpectAtom(&name) || !dec.ExpectSP() || !dec.ExpectNumber64(&resData.Usage) || !dec.ExpectSP() || !dec.ExpectNumber64(&resData.Limit) {
			return fmt.Errorf("in quota-resource: %v", dec.Err())
		}
		data.Resources[imap.QuotaResourceType(name)] = resData // 将资源数据添加到列表
		return nil
	})
	return &data, err
}

// readQuotaRoot 读取 QUOTAROOT 响应。
func readQuotaRoot(dec *imapwire.Decoder) (mailbox string, roots []string, err error) {
	if !dec.ExpectMailbox(&mailbox) { // 读取邮箱名称
		return "", nil, dec.Err()
	}
	for dec.SP() {
		var root string
		if !dec.ExpectAString(&root) { // 读取根命名空间
			return "", nil, dec.Err()
		}
		roots = append(roots, root) // 添加到根命名空间列表
	}
	return mailbox, roots, nil
}
