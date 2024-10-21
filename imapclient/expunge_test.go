package imapclient_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestExpunge(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected) // 创建客户端和服务器对
	defer client.Close()                                             // 延迟关闭客户端
	defer server.Close()                                             // 延迟关闭服务器

	// 发送 EXPUNGE 命令并收集序列号
	seqNums, err := client.Expunge().Collect()
	if err != nil {
		t.Fatalf("Expunge() = %v", err) // 检查 EXPUNGE 是否出错
	} else if len(seqNums) != 0 {
		t.Errorf("Expunge().Collect() = %v, want []", seqNums) // 期望返回空列表
	}

	seqSet := imap.SeqSetNum(1) // 创建序列集
	storeFlags := imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,            // 添加操作
		Flags: []imap.Flag{imap.FlagDeleted}, // 设置为已删除标志
	}
	if err := client.Store(seqSet, &storeFlags, nil).Close(); err != nil {
		t.Fatalf("Store() = %v", err) // 检查 Store 是否出错
	}

	// 发送 EXPUNGE 命令并收集序列号
	seqNums, err = client.Expunge().Collect()
	if err != nil {
		t.Fatalf("Expunge() = %v", err) // 检查 EXPUNGE 是否出错
	} else if len(seqNums) != 1 || seqNums[0] != 1 {
		t.Errorf("Expunge().Collect() = %v, want [1]", seqNums) // 期望返回 [1]
	}
}
