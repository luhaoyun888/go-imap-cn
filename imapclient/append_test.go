package imapclient_test

import (
	"testing"

	"github.com/luhaoyun888/go-imap-cn"
)

// TestAppend 测试 APPEND 命令。
func TestAppend(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close() // 关闭客户端
	defer server.Close() // 关闭服务器

	body := "这是一条测试消息。" // 消息内容

	// 创建 APPEND 命令
	appendCmd := client.Append("INBOX", int64(len(body)), nil)
	if _, err := appendCmd.Write([]byte(body)); err != nil {
		t.Fatalf("AppendCommand.Write() 出错: %v", err) // 写入消息时出错
	}
	if err := appendCmd.Close(); err != nil {
		t.Fatalf("AppendCommand.Close() 出错: %v", err) // 关闭命令时出错
	}
	if _, err := appendCmd.Wait(); err != nil {
		t.Fatalf("AppendCommand.Wait() 出错: %v", err) // 等待命令响应时出错
	}

	// TODO: 获取消息并检查内容
}
