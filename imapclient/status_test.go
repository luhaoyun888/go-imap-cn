package imapclient_test

import (
	"reflect"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestStatus(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated) // 创建客户端和服务器对
	defer client.Close()                                                  // 关闭客户端
	defer server.Close()                                                  // 关闭服务器

	// 设置状态选项
	options := imap.StatusOptions{
		NumMessages: true, // 请求消息数量
		NumUnseen:   true, // 请求未读消息数量
	}

	// 获取邮箱状态数据
	data, err := client.Status("INBOX", &options).Wait()
	if err != nil {
		t.Fatalf("Status() = %v", err) // 失败时打印错误信息
	}

	// 定义期望的结果
	wantNumMessages := uint32(1) // 期望的消息数量
	wantNumUnseen := uint32(1)   // 期望的未读消息数量
	want := &imap.StatusData{
		Mailbox:     "INBOX",          // 期望的邮箱名称
		NumMessages: &wantNumMessages, // 期望的消息数量指针
		NumUnseen:   &wantNumUnseen,   // 期望的未读消息数量指针
	}

	// 比较实际数据与期望结果
	if !reflect.DeepEqual(data, want) {
		t.Errorf("Status() = %#v but want %#v", data, want) // 如果不相等则打印错误信息
	}
}
