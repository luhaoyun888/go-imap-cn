package imapclient

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// Namespace 发送 NAMESPACE 命令。
//
// 此命令要求支持 IMAP4rev2 或 NAMESPACE 扩展。
func (c *Client) Namespace() *NamespaceCommand {
	cmd := &NamespaceCommand{}
	c.beginCommand("NAMESPACE", cmd).end() // 开始并结束命令
	return cmd
}

// handleNamespace 处理 NAMESPACE 响应。
func (c *Client) handleNamespace() error {
	data, err := readNamespaceResponse(c.dec) // 读取 NAMESPACE 响应
	if err != nil {
		return fmt.Errorf("in namespace-response: %v", err)
	}
	if cmd := findPendingCmdByType[*NamespaceCommand](c); cmd != nil {
		cmd.data = *data // 保存响应数据
	}
	return nil
}

// NamespaceCommand 是 NAMESPACE 命令的结构体。
type NamespaceCommand struct {
	commandBase
	data imap.NamespaceData // 存储命令返回的数据
}

// Wait 等待命令完成，并返回 NAMESPACE 数据。
func (cmd *NamespaceCommand) Wait() (*imap.NamespaceData, error) {
	return &cmd.data, cmd.wait() // 返回数据和等待结果
}

// readNamespaceResponse 读取 NAMESPACE 响应。
func readNamespaceResponse(dec *imapwire.Decoder) (*imap.NamespaceData, error) {
	var (
		data imap.NamespaceData
		err  error
	)

	data.Personal, err = readNamespace(dec) // 读取个人命名空间
	if err != nil {
		return nil, err
	}

	if !dec.ExpectSP() {
		return nil, dec.Err()
	}

	data.Other, err = readNamespace(dec) // 读取其他命名空间
	if err != nil {
		return nil, err
	}

	if !dec.ExpectSP() {
		return nil, dec.Err()
	}

	data.Shared, err = readNamespace(dec) // 读取共享命名空间
	if err != nil {
		return nil, err
	}

	return &data, nil
}

// readNamespace 读取命名空间描述符。
func readNamespace(dec *imapwire.Decoder) ([]imap.NamespaceDescriptor, error) {
	var l []imap.NamespaceDescriptor
	err := dec.ExpectNList(func() error {
		descr, err := readNamespaceDescr(dec) // 读取命名空间描述符
		if err != nil {
			return fmt.Errorf("in namespace-descr: %v", err)
		}
		l = append(l, *descr) // 添加到列表中
		return nil
	})
	return l, err
}

// readNamespaceDescr 读取命名空间描述符。
func readNamespaceDescr(dec *imapwire.Decoder) (*imap.NamespaceDescriptor, error) {
	var descr imap.NamespaceDescriptor

	if !dec.ExpectSpecial('(') || !dec.ExpectString(&descr.Prefix) || !dec.ExpectSP() {
		return nil, dec.Err()
	}

	var err error
	descr.Delim, err = readDelim(dec) // 读取分隔符
	if err != nil {
		return nil, err
	}

	// 跳过命名空间响应扩展
	for dec.SP() {
		if !dec.DiscardValue() {
			return nil, dec.Err()
		}
	}

	if !dec.ExpectSpecial(')') {
		return nil, dec.Err()
	}

	return &descr, nil
}
