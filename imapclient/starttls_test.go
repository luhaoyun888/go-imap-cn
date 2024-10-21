package imapclient_test

import (
	"crypto/tls"
	"testing"

	"github.com/luhaoyun888/go-imap-cn/imapclient"
)

// TestStartTLS 测试 STARTTLS 功能
func TestStartTLS(t *testing.T) {
	conn, server := newMemClientServerPair(t) // 创建一个内存客户端和服务器对
	defer conn.Close()                        // 关闭连接
	defer server.Close()                      // 关闭服务器

	options := imapclient.Options{
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, // TLS 配置，允许不安全的连接
	}
	client, err := imapclient.NewStartTLS(conn, &options) // 创建新的 STARTTLS 客户端
	if err != nil {
		t.Fatalf("NewStartTLS() = %v", err) // 如果创建失败，输出错误信息
	}
	defer client.Close() // 关闭客户端

	if err := client.Noop().Wait(); err != nil { // 发送 NOOP 命令并等待响应
		t.Fatalf("Noop().Wait() = %v", err) // 如果 NOOP 命令失败，输出错误信息
	}
}
