package imapclient_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestESearch(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected) // 创建客户端和服务器配对
	defer client.Close()                                             // 测试结束后关闭客户端
	defer server.Close()                                             // 测试结束后关闭服务器

	if !client.Caps().Has(imap.CapESearch) { // 检查服务器是否支持 ESEARCH
		t.Skip("服务器不支持 ESEARCH")
	}

	criteria := imap.SearchCriteria{ // 定义搜索标准
		Header: []imap.SearchCriteriaHeaderField{{
			Key:   "Message-Id",                    // 头部字段：消息 ID
			Value: "<191101702316132@example.com>", // 消息 ID 的值
		}},
	}
	options := imap.SearchOptions{ // 定义搜索选项
		ReturnCount: true, // 返回计数
	}
	data, err := client.Search(&criteria, &options).Wait() // 执行搜索并等待结果
	if err != nil {
		t.Fatalf("Search().Wait() = %v", err) // 如果有错误，记录错误信息
	}
	if want := uint32(1); data.Count != want { // 检查返回的计数是否符合预期
		t.Errorf("Count = %v, want %v", data.Count, want)
	}
}
