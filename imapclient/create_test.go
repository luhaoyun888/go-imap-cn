package imapclient_test

import (
	"testing"

	"github.com/luhaoyun888/go-imap-cn"
)

// testCreate 测试 CREATE 命令的实现。
// 参数：
//
//	t - 测试对象。
//	name - 要创建的邮箱名称。
//	utf8Accept - 是否接受 UTF-8 编码的邮箱名称。
func testCreate(t *testing.T, name string, utf8Accept bool) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated) // 创建客户端和服务器对
	defer client.Close()                                                  // 确保在测试结束时关闭客户端
	defer server.Close()                                                  // 确保在测试结束时关闭服务器

	if utf8Accept { // 如果接受 UTF-8 编码
		if !client.Caps().Has(imap.CapUTF8Accept) { // 检查客户端是否支持 UTF8=ACCEPT
			t.Skipf("缺少 UTF8=ACCEPT 支持") // 如果不支持，跳过测试
		}
		if data, err := client.Enable(imap.CapUTF8Accept).Wait(); err != nil { // 启用 UTF8=ACCEPT
			t.Fatalf("Enable(CapUTF8Accept) = %v", err) // 启用失败，报告错误
		} else if !data.Caps.Has(imap.CapUTF8Accept) { // 检查服务器是否允许启用
			t.Fatalf("服务器拒绝启用 UTF8=ACCEPT") // 如果不允许，报告错误
		}
	}

	if err := client.Create(name, nil).Wait(); err != nil { // 尝试创建邮箱
		t.Fatalf("Create() = %v", err) // 创建失败，报告错误
	}

	listCmd := client.List("", name, nil) // 列出邮箱
	mailboxes, err := listCmd.Collect()   // 收集邮箱列表
	if err != nil {
		t.Errorf("List() = %v", err) // 列出失败，报告错误
	} else if len(mailboxes) != 1 || mailboxes[0].Mailbox != name { // 检查邮箱列表是否正确
		t.Errorf("List() = %v, 希望有一个条目且名称正确", mailboxes)
	}
}

// TestCreate 测试 CREATE 命令的各种情况。
func TestCreate(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		testCreate(t, "Test mailbox", false) // 基本测试
	})

	t.Run("unicode_utf7", func(t *testing.T) {
		testCreate(t, "Cafè", false) // 测试 UTF-7 编码的 Unicode 字符
	})
	t.Run("unicode_utf8", func(t *testing.T) {
		testCreate(t, "Cafè", true) // 测试 UTF-8 编码的 Unicode 字符
	})

	// '&' 是 UTF-7 转义字符
	t.Run("ampersand_utf7", func(t *testing.T) {
		testCreate(t, "Angus & Julia", false) // 测试 UTF-7 编码中包含 '&' 字符的情况
	})
	t.Run("ampersand_utf8", func(t *testing.T) {
		testCreate(t, "Angus & Julia", true) // 测试 UTF-8 编码中包含 '&' 字符的情况
	})
}
