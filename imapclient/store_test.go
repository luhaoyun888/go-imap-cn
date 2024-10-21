package imapclient_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

// TestStore 测试 Store 方法
func TestStore(t *testing.T) {
	// 创建客户端和服务器对
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close() // 关闭客户端
	defer server.Close() // 关闭服务器

	seqSet := imap.SeqSetNum(1) // 创建序列集，指定消息序列号为 1
	storeFlags := imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,            // 操作类型：添加标志
		Flags: []imap.Flag{imap.FlagDeleted}, // 要添加的标志：已删除
	}

	// 执行 Store 操作并收集结果
	msgs, err := client.Store(seqSet, &storeFlags, nil).Collect()
	if err != nil {
		t.Fatalf("Store().Collect() = %v", err) // 处理错误
	} else if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %v, want %v", len(msgs), 1) // 检查返回消息数量
	}

	msg := msgs[0] // 获取第一个消息
	if msg.SeqNum != 1 {
		t.Errorf("msg.SeqNum = %v, want %v", msg.SeqNum, 1) // 检查消息序列号
	}

	found := false
	// 检查消息标志中是否包含已删除标志
	for _, f := range msg.Flags {
		if f == imap.FlagDeleted {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("msg.Flags 中缺少已删除标志: %v", msg.Flags) // 如果未找到已删除标志，记录错误
	}
}
