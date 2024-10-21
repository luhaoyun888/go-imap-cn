package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// Capability 发送 CAPABILITY 命令。
func (c *Client) Capability() *CapabilityCommand {
	cmd := &CapabilityCommand{}
	c.beginCommand("CAPABILITY", cmd).end() // 开始 CAPABILITY 命令并结束
	return cmd
}

// handleCapability 处理 CAPABILITY 命令的响应。
func (c *Client) handleCapability() error {
	caps, err := readCapabilities(c.dec) // 读取能力信息
	if err != nil {
		return err
	}
	c.setCaps(caps) // 设置能力
	if cmd := findPendingCmdByType[*CapabilityCommand](c); cmd != nil {
		cmd.caps = caps // 将能力信息赋值给命令
	}
	return nil
}

// CapabilityCommand 是一个 CAPABILITY 命令。
type CapabilityCommand struct {
	commandBase
	caps imap.CapSet // 能力集合
}

// Wait 等待 CAPABILITY 命令的响应并返回能力集合。
func (cmd *CapabilityCommand) Wait() (imap.CapSet, error) {
	err := cmd.wait()    // 等待命令响应
	return cmd.caps, err // 返回能力集合和错误信息
}

// readCapabilities 读取能力数据并返回能力集合。
func readCapabilities(dec *imapwire.Decoder) (imap.CapSet, error) {
	caps := make(imap.CapSet) // 创建能力集合
	for dec.SP() {
		var name string
		if !dec.ExpectAtom(&name) { // 期望一个原子
			return caps, fmt.Errorf("在能力数据中: %v", dec.Err())
		}
		caps[imap.Cap(name)] = struct{}{} // 将能力添加到集合中
	}
	return caps, nil
}
