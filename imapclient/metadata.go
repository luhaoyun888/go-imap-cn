package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// GetMetadataDepth 表示获取元数据的深度。
type GetMetadataDepth int

const (
	GetMetadataDepthZero     GetMetadataDepth = 0  // 不获取任何元数据
	GetMetadataDepthOne      GetMetadataDepth = 1  // 仅获取一级元数据
	GetMetadataDepthInfinity GetMetadataDepth = -1 // 获取所有元数据
)

// String 返回 GetMetadataDepth 的字符串表示。
func (depth GetMetadataDepth) String() string {
	switch depth {
	case GetMetadataDepthZero:
		return "0"
	case GetMetadataDepthOne:
		return "1"
	case GetMetadataDepthInfinity:
		return "infinity"
	default:
		panic(fmt.Errorf("imapclient: unknown GETMETADATA depth %d", depth))
	}
}

// GetMetadataOptions 包含 GETMETADATA 命令的选项。
type GetMetadataOptions struct {
	MaxSize *uint32          // 最大大小选项
	Depth   GetMetadataDepth // 获取深度选项
}

// names 返回 GETMETADATA 选项的名称列表。
func (options *GetMetadataOptions) names() []string {
	if options == nil {
		return nil
	}
	var l []string
	if options.MaxSize != nil {
		l = append(l, "MAXSIZE") // 添加最大大小选项
	}
	if options.Depth != GetMetadataDepthZero {
		l = append(l, "DEPTH") // 添加深度选项
	}
	return l
}

// GetMetadata 发送 GETMETADATA 命令。
//
// 此命令要求支持 METADATA 或 METADATA-SERVER 扩展。
func (c *Client) GetMetadata(mailbox string, entries []string, options *GetMetadataOptions) *GetMetadataCommand {
	cmd := &GetMetadataCommand{mailbox: mailbox}
	enc := c.beginCommand("GETMETADATA", cmd)
	enc.SP().Mailbox(mailbox)
	if opts := options.names(); len(opts) > 0 {
		enc.SP().List(len(opts), func(i int) {
			opt := opts[i]
			enc.Atom(opt).SP()
			switch opt {
			case "MAXSIZE":
				enc.Number(*options.MaxSize) // 设置最大大小
			case "DEPTH":
				enc.Atom(options.Depth.String()) // 设置获取深度
			default:
				panic(fmt.Errorf("imapclient: unknown GETMETADATA option %q", opt))
			}
		})
	}
	enc.SP().List(len(entries), func(i int) {
		enc.String(entries[i]) // 添加要获取的条目
	})
	enc.end()
	return cmd
}

// SetMetadata 发送 SETMETADATA 命令。
//
// 要删除条目，请将其设置为 nil。
//
// 此命令要求支持 METADATA 或 METADATA-SERVER 扩展。
func (c *Client) SetMetadata(mailbox string, entries map[string]*[]byte) *Command {
	cmd := &Command{}
	enc := c.beginCommand("SETMETADATA", cmd)
	enc.SP().Mailbox(mailbox).SP().Special('(')
	i := 0
	for k, v := range entries {
		if i > 0 {
			enc.SP()
		}
		enc.String(k).SP() // 设置条目键
		if v == nil {
			enc.NIL() // 设置为 nil
		} else {
			enc.String(string(*v)) // TODO: 根据需要使用字面量
		}
		i++
	}
	enc.Special(')') // 结束命令
	enc.end()
	return cmd
}

// handleMetadata 处理元数据响应。
func (c *Client) handleMetadata() error {
	data, err := readMetadataResp(c.dec) // 读取元数据响应
	if err != nil {
		return fmt.Errorf("in metadata-resp: %v", err)
	}

	cmd := c.findPendingCmdFunc(func(anyCmd command) bool {
		cmd, ok := anyCmd.(*GetMetadataCommand)
		return ok && cmd.mailbox == data.Mailbox
	})
	if cmd != nil && len(data.EntryValues) > 0 {
		cmd := cmd.(*GetMetadataCommand)
		cmd.data.Mailbox = data.Mailbox
		if cmd.data.Entries == nil {
			cmd.data.Entries = make(map[string]*[]byte)
		}
		// 服务器可能会为单个 METADATA 命令发送多个响应
		for k, v := range data.EntryValues {
			cmd.data.Entries[k] = v
		}
	} else if handler := c.options.unilateralDataHandler().Metadata; handler != nil && len(data.EntryList) > 0 {
		handler(data.Mailbox, data.EntryList)
	}

	return nil
}

// GetMetadataCommand 是 GETMETADATA 命令的结构体。
type GetMetadataCommand struct {
	commandBase
	mailbox string
	data    GetMetadataData
}

// Wait 等待 GETMETADATA 命令完成，并返回结果数据。
func (cmd *GetMetadataCommand) Wait() (*GetMetadataData, error) {
	return &cmd.data, cmd.wait()
}

// GetMetadataData 是 GETMETADATA 命令返回的数据。
type GetMetadataData struct {
	Mailbox string
	Entries map[string]*[]byte // 条目数据
}

type metadataResp struct {
	Mailbox     string
	EntryList   []string
	EntryValues map[string]*[]byte // 条目值
}

// readMetadataResp 读取元数据响应。
func readMetadataResp(dec *imapwire.Decoder) (*metadataResp, error) {
	var data metadataResp

	if !dec.ExpectMailbox(&data.Mailbox) || !dec.ExpectSP() {
		return nil, dec.Err()
	}

	isList, err := dec.List(func() error {
		var name string
		if !dec.ExpectAString(&name) || !dec.ExpectSP() {
			return dec.Err()
		}

		// TODO: 以 []byte 解码
		var (
			value *[]byte
			s     string
		)
		if dec.String(&s) || dec.Literal(&s) {
			b := []byte(s)
			value = &b
		} else if !dec.ExpectNIL() {
			return dec.Err()
		}

		if data.EntryValues == nil {
			data.EntryValues = make(map[string]*[]byte)
		}
		data.EntryValues[name] = value
		return nil
	})
	if err != nil {
		return nil, err
	} else if !isList {
		var name string
		if !dec.ExpectAString(&name) {
			return nil, dec.Err()
		}
		data.EntryList = append(data.EntryList, name)

		for dec.SP() {
			if !dec.ExpectAString(&name) {
				return nil, dec.Err()
			}
			data.EntryList = append(data.EntryList, name)
		}
	}

	return &data, nil
}
