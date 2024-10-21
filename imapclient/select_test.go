package imapclient_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestSelect(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated) // 创建客户端和服务器
	defer client.Close()                                                  // 确保关闭客户端连接
	defer server.Close()                                                  // 确保关闭服务器连接

	data, err := client.Select("INBOX", nil).Wait() // 选择 INBOX 邮箱并等待响应
	if err != nil {
		t.Fatalf("Select() = %v", err) // 如果出现错误，记录失败
	} else if data.NumMessages != 1 { // 检查消息数量是否符合预期
		t.Errorf("SelectData.NumMessages = %v, want %v", data.NumMessages, 1) // 如果不符合，记录错误
	}
}
