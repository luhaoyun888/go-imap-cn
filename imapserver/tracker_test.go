package imapserver_test

import (
	"testing"

	"github.com/emersion/go-imap/v2/imapserver"
)

// trackerUpdate 结构体用于跟踪邮件更新的状态
type trackerUpdate struct {
	expunge     uint32 // 要删除的邮件序号
	numMessages uint32 // 当前邮件数量
}

// sessionTrackerSeqNumTests 存储多个测试用例的名称、待处理更新、客户端序列号和服务器序列号
var sessionTrackerSeqNumTests = []struct {
	name                       string // 测试用例名称
	pending                    []trackerUpdate // 待处理的邮件更新
	clientSeqNum, serverSeqNum uint32 // 客户端和服务器的序列号
}{
	{
		name:         "无操作",
		pending:      nil,
		clientSeqNum: 20,
		serverSeqNum: 20,
	},
	{
		name:         "无操作_最后",
		pending:      nil,
		clientSeqNum: 42,
		serverSeqNum: 42,
	},
	{
		name:         "无操作_客户端超出",
		pending:      nil,
		clientSeqNum: 43,
		serverSeqNum: 0,
	},
	{
		name:         "无操作_服务器超出",
		pending:      nil,
		clientSeqNum: 0,
		serverSeqNum: 43,
	},
	{
		name:         "删除相等",
		pending:      []trackerUpdate{{expunge: 20}},
		clientSeqNum: 20,
		serverSeqNum: 0,
	},
	{
		name:         "删除小于",
		pending:      []trackerUpdate{{expunge: 20}},
		clientSeqNum: 10,
		serverSeqNum: 10,
	},
	{
		name:         "删除大于",
		pending:      []trackerUpdate{{expunge: 10}},
		clientSeqNum: 20,
		serverSeqNum: 19,
	},
	{
		name:         "添加相等",
		pending:      []trackerUpdate{{numMessages: 43}},
		clientSeqNum: 0,
		serverSeqNum: 43,
	},
	{
		name:         "添加小于",
		pending:      []trackerUpdate{{numMessages: 43}},
		clientSeqNum: 42,
		serverSeqNum: 42,
	},
	{
		name: "删除_添加",
		pending: []trackerUpdate{
			{expunge: 42},
			{numMessages: 42},
		},
		clientSeqNum: 42,
		serverSeqNum: 0,
	},
	{
		name: "删除_添加",
		pending: []trackerUpdate{
			{expunge: 42},
			{numMessages: 42},
		},
		clientSeqNum: 0,
		serverSeqNum: 42,
	},
	{
		name: "添加_删除",
		pending: []trackerUpdate{
			{numMessages: 43},
			{expunge: 42},
		},
		clientSeqNum: 42,
		serverSeqNum: 0,
	},
	{
		name: "添加_删除",
		pending: []trackerUpdate{
			{numMessages: 43},
			{expunge: 42},
		},
		clientSeqNum: 0,
		serverSeqNum: 42,
	},
	{
		name: "多个删除_中间",
		pending: []trackerUpdate{
			{expunge: 3},
			{expunge: 1},
		},
		clientSeqNum: 2,
		serverSeqNum: 1,
	},
	{
		name: "多个删除_之后",
		pending: []trackerUpdate{
			{expunge: 3},
			{expunge: 1},
		},
		clientSeqNum: 4,
		serverSeqNum: 2,
	},
}

// TestSessionTracker 测试邮件会话跟踪器
func TestSessionTracker(t *testing.T) {
	for _, tc := range sessionTrackerSeqNumTests {
		tc := tc // 捕获范围变量
		t.Run(tc.name, func(t *testing.T) {
			mboxTracker := imapserver.NewMailboxTracker(42) // 创建新的邮箱跟踪器
			sessTracker := mboxTracker.NewSession() // 创建新的会话跟踪器
			for _, update := range tc.pending {
				switch {
				case update.expunge != 0:
					mboxTracker.QueueExpunge(update.expunge) // 队列中添加待删除的邮件序号
				case update.numMessages != 0:
					mboxTracker.QueueNumMessages(update.numMessages) // 队列中添加当前邮件数量
				}
			}

			serverSeqNum := sessTracker.DecodeSeqNum(tc.clientSeqNum) // 解码客户端序列号
			if tc.clientSeqNum != 0 && serverSeqNum != tc.serverSeqNum {
				t.Errorf("DecodeSeqNum(%v): got %v, want %v", tc.clientSeqNum, serverSeqNum, tc.serverSeqNum)
			}

			clientSeqNum := sessTracker.EncodeSeqNum(tc.serverSeqNum) // 编码服务器序列号
			if tc.serverSeqNum != 0 && clientSeqNum != tc.clientSeqNum {
				t.Errorf("EncodeSeqNum(%v): got %v, want %v", tc.serverSeqNum, clientSeqNum, tc.clientSeqNum)
			}
		})
	}
}
