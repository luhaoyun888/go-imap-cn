package imapclient_test

import (
	"reflect"
	"testing"

	"github.com/luhaoyun888/go-imap-cn"
)

// TestList 测试 List 命令。
func TestList(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated)
	defer client.Close() // 确保客户端关闭
	defer server.Close() // 确保服务器关闭

	options := imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true, // 请求返回邮件数量
		},
	}
	mailboxes, err := client.List("", "%", &options).Collect() // 获取邮箱列表
	if err != nil {
		t.Fatalf("List() = %v", err) // 检查 List 命令是否成功
	}

	if len(mailboxes) != 1 { // 检查返回的邮箱数量
		t.Fatalf("List() returned %v mailboxes, want 1", len(mailboxes))
	}
	mbox := mailboxes[0] // 获取第一个邮箱

	wantNumMessages := uint32(1) // 期望的邮件数量
	want := &imap.ListData{
		Delim:   '/',     // 邮箱分隔符
		Mailbox: "INBOX", // 邮箱名称
		Status: &imap.StatusData{
			Mailbox:     "INBOX",          // 邮箱名称
			NumMessages: &wantNumMessages, // 期望的邮件数量
		},
	}
	if !reflect.DeepEqual(mbox, want) { // 检查返回的邮箱数据是否与期望相等
		t.Errorf("got %#v but want %#v", mbox, want) // 输出不匹配的错误信息
	}
}
