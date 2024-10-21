package imapclient_test

import (
	"testing"

	"github.com/luhaoyun888/go-imap-cn"
)

// 测试用例结构体
var testCases = []struct {
	name                  string                 // 测试用例名称
	mailbox               string                 // 邮箱名称
	setRightsModification imap.RightModification // 权限修改类型
	setRights             imap.RightSet          // 设置的权限集
	expectedRights        imap.RightSet          // 期望的权限集
	execStatusCmd         bool                   // 是否执行状态命令
}{
	{
		name:                  "收件箱",
		mailbox:               "INBOX",
		setRightsModification: imap.RightModificationReplace,  // 替换权限
		setRights:             imap.RightSet("akxeilprwtscd"), // 设置的权限
		expectedRights:        imap.RightSet("akxeilprwtscd"), // 期望的权限
	},
	{
		name:                  "自定义文件夹",
		mailbox:               "MyFolder",
		setRightsModification: imap.RightModificationReplace, // 替换权限
		setRights:             imap.RightSet("ailw"),         // 设置的权限
		expectedRights:        imap.RightSet("ailw"),         // 期望的权限
	},
	{
		name:                  "自定义子文件夹",
		mailbox:               "MyFolder.Child",
		setRightsModification: imap.RightModificationReplace, // 替换权限
		setRights:             imap.RightSet("aelrwtd"),      // 设置的权限
		expectedRights:        imap.RightSet("aelrwtd"),      // 期望的权限
	},
	{
		name:                  "添加权限",
		mailbox:               "MyFolder",
		setRightsModification: imap.RightModificationAdd, // 添加权限
		setRights:             imap.RightSet("rwi"),      // 设置的权限
		expectedRights:        imap.RightSet("ailwr"),    // 期望的权限
	},
	{
		name:                  "移除权限",
		mailbox:               "MyFolder",
		setRightsModification: imap.RightModificationRemove, // 移除权限
		setRights:             imap.RightSet("iwc"),         // 设置的权限
		expectedRights:        imap.RightSet("alr"),         // 期望的权限
	},
	{
		name:                  "空权限",
		mailbox:               "MyFolder.Child",
		setRightsModification: imap.RightModificationReplace, // 替换权限
		setRights:             imap.RightSet("a"),            // 设置的权限
		expectedRights:        imap.RightSet("a"),            // 期望的权限
	},
}

// TestACL 对 SetACL、GetACL 和 MyRights 命令进行测试。
func TestACL(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated)
	defer client.Close()
	defer server.Close()

	if !client.Caps().Has(imap.CapACL) {
		t.Skipf("服务器不支持 ACL")
	}

	// 创建文件夹 MyFolder
	if err := client.Create("MyFolder", nil).Wait(); err != nil {
		t.Fatalf("创建 MyFolder 时出错: %v", err)
	}

	// 创建子文件夹 MyFolder/Child
	if err := client.Create("MyFolder/Child", nil).Wait(); err != nil {
		t.Fatalf("创建 MyFolder/Child 时出错: %v", err)
	}

	// 逐个执行测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 执行 SETACL 命令
			err := client.SetACL(tc.mailbox, testUsername, tc.setRightsModification, tc.setRights).Wait()
			if err != nil {
				t.Errorf("SetACL().Wait() 出错: %v", err)
			}

			// 执行 GETACL 命令以重置服务器上的缓存
			getACLData, err := client.GetACL(tc.mailbox).Wait()
			if err != nil {
				t.Errorf("GetACL().Wait() 出错: %v", err)
			}

			if !tc.expectedRights.Equal(getACLData.Rights[testUsername]) {
				t.Errorf("GETACL 返回的权限错误; 期望: %s, 实际: %s", tc.expectedRights, getACLData.Rights[testUsername])
			}

			// 执行 MYRIGHTS 命令
			myRightsData, err := client.MyRights(tc.mailbox).Wait()
			if err != nil {
				t.Errorf("MyRights().Wait() 出错: %v", err)
			}

			if !tc.expectedRights.Equal(myRightsData.Rights) {
				t.Errorf("MYRIGHTS 返回的权限错误; 期望: %s, 实际: %s", tc.expectedRights, myRightsData.Rights)
			}
		})
	}

	t.Run("不存在的邮箱", func(t *testing.T) {
		if client.SetACL("BibiMailbox", testUsername, imap.RightModificationReplace, nil).Wait() == nil {
			t.Errorf("期望出现错误")
		}
	})
}
