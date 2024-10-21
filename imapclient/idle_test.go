package imapclient_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

// TestIdle 测试 IDLE 命令。
func TestIdle(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close() // 确保客户端关闭
	defer server.Close() // 确保服务器关闭

	idleCmd, err := client.Idle() // 发送 IDLE 命令
	if err != nil {
		t.Fatalf("Idle() = %v", err) // 检查 IDLE 命令是否成功
	}
	// TODO: 测试单方面更新
	if err := idleCmd.Close(); err != nil { // 关闭 IDLE 命令
		t.Errorf("Close() = %v", err) // 检查关闭是否成功
	}
}

// TestIdle_closedConn 测试关闭连接时的 IDLE 命令。
func TestIdle_closedConn(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close() // 确保客户端关闭
	defer server.Close() // 确保服务器关闭

	idleCmd, err := client.Idle() // 发送 IDLE 命令
	if err != nil {
		t.Fatalf("Idle() = %v", err) // 检查 IDLE 命令是否成功
	}
	defer idleCmd.Close() // 确保 IDLE 命令关闭

	if err := client.Close(); err != nil { // 关闭客户端
		t.Fatalf("client.Close() = %v", err) // 检查关闭是否成功
	}

	if err := idleCmd.Wait(); err == nil { // 等待 IDLE 命令完成
		t.Errorf("IdleCommand.Wait() = nil, want an error") // 检查是否返回错误
	}
}
