package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// ID 发送 ID 命令。
//
// ID 命令在 RFC 2971 中引入。它需要支持 ID 扩展。
//
// 一个 ID 命令的示例：
//
//	ID ("name" "go-imap" "version" "1.0" "os" "Linux" "os-version" "7.9.4" "vendor" "Yahoo")
func (c *Client) ID(idData *imap.IDData) *IDCommand {
	cmd := &IDCommand{}
	enc := c.beginCommand("ID", cmd)

	if idData == nil {
		enc.SP().NIL() // 如果没有提供 ID 数据，发送 NIL
		enc.end()
		return cmd
	}

	enc.SP().Special('(')
	isFirstKey := true
	if idData.Name != "" {
		addIDKeyValue(enc, &isFirstKey, "name", idData.Name)
	}
	if idData.Version != "" {
		addIDKeyValue(enc, &isFirstKey, "version", idData.Version)
	}
	if idData.OS != "" {
		addIDKeyValue(enc, &isFirstKey, "os", idData.OS)
	}
	if idData.OSVersion != "" {
		addIDKeyValue(enc, &isFirstKey, "os-version", idData.OSVersion)
	}
	if idData.Vendor != "" {
		addIDKeyValue(enc, &isFirstKey, "vendor", idData.Vendor)
	}
	if idData.SupportURL != "" {
		addIDKeyValue(enc, &isFirstKey, "support-url", idData.SupportURL)
	}
	if idData.Address != "" {
		addIDKeyValue(enc, &isFirstKey, "address", idData.Address)
	}
	if idData.Date != "" {
		addIDKeyValue(enc, &isFirstKey, "date", idData.Date)
	}
	if idData.Command != "" {
		addIDKeyValue(enc, &isFirstKey, "command", idData.Command)
	}
	if idData.Arguments != "" {
		addIDKeyValue(enc, &isFirstKey, "arguments", idData.Arguments)
	}
	if idData.Environment != "" {
		addIDKeyValue(enc, &isFirstKey, "environment", idData.Environment)
	}

	enc.Special(')')
	enc.end()
	return cmd
}

// addIDKeyValue 添加 ID 的键值对
func addIDKeyValue(enc *commandEncoder, isFirstKey *bool, key, value string) {
	if isFirstKey == nil {
		panic("isFirstKey cannot be nil") // isFirstKey 不能为空
	} else if !*isFirstKey {
		enc.SP().Quoted(key).SP().Quoted(value) // 如果不是第一个键值对，添加空格
	} else {
		enc.Quoted(key).SP().Quoted(value) // 如果是第一个键值对，不添加空格
	}
	*isFirstKey = false // 设置为非第一个键值对
}

func (c *Client) handleID() error {
	data, err := c.readID(c.dec)
	if err != nil {
		return fmt.Errorf("in id: %v", err)
	}

	if cmd := findPendingCmdByType[*IDCommand](c); cmd != nil {
		cmd.data = *data // 将数据保存到命令中
	}

	return nil
}

// readID 从解码器中读取 ID 数据
func (c *Client) readID(dec *imapwire.Decoder) (*imap.IDData, error) {
	var data = imap.IDData{}

	if !dec.ExpectSP() {
		return nil, dec.Err()
	}

	if dec.ExpectNIL() {
		return &data, nil // 如果是 NIL，返回空数据
	}

	currKey := ""
	err := dec.ExpectList(func() error {
		var keyOrValue string
		if !dec.String(&keyOrValue) {
			return fmt.Errorf("in id key-val list: %v", dec.Err())
		}

		if currKey == "" {
			currKey = keyOrValue // 记录当前键
			return nil
		}

		// 根据当前键设置值
		switch currKey {
		case "name":
			data.Name = keyOrValue
		case "version":
			data.Version = keyOrValue
		case "os":
			data.OS = keyOrValue
		case "os-version":
			data.OSVersion = keyOrValue
		case "vendor":
			data.Vendor = keyOrValue
		case "support-url":
			data.SupportURL = keyOrValue
		case "address":
			data.Address = keyOrValue
		case "date":
			data.Date = keyOrValue
		case "command":
			data.Command = keyOrValue
		case "arguments":
			data.Arguments = keyOrValue
		case "environment":
			data.Environment = keyOrValue
		default:
			// 忽略未知的键
			// Yahoo 服务器发送 "host" 和 "remote-host" 键
			// 这些在 RFC 2971 中未定义
		}
		currKey = "" // 重置当前键

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &data, nil
}

// IDCommand 表示 ID 命令。
type IDCommand struct {
	commandBase
	data imap.IDData // 存储 ID 数据
}

// Wait 等待命令完成，并返回 ID 数据。
func (r *IDCommand) Wait() (*imap.IDData, error) {
	return &r.data, r.wait() // 返回 ID 数据和命令等待结果
}
