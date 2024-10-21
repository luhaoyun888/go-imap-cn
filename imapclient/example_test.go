package imapclient_test

import (
	"io"
	"log"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// ExampleClient 展示如何使用 Client 连接到 IMAP 服务器并进行基本操作。
func ExampleClient() {
	c, err := imapclient.DialTLS("mail.example.org:993", nil) // 连接到 IMAP 服务器
	if err != nil {
		log.Fatalf("无法连接到 IMAP 服务器: %v", err)
	}
	defer c.Close() // 确保关闭连接

	if err := c.Login("root", "asdf").Wait(); err != nil { // 登录
		log.Fatalf("登录失败: %v", err)
	}

	mailboxes, err := c.List("", "%", nil).Collect() // 列出邮箱
	if err != nil {
		log.Fatalf("列出邮箱失败: %v", err)
	}
	log.Printf("找到 %v 个邮箱", len(mailboxes))
	for _, mbox := range mailboxes {
		log.Printf(" - %v", mbox.Mailbox) // 输出邮箱名称
	}

	selectedMbox, err := c.Select("INBOX", nil).Wait() // 选择收件箱
	if err != nil {
		log.Fatalf("选择 INBOX 失败: %v", err)
	}
	log.Printf("INBOX 中包含 %v 封邮件", selectedMbox.NumMessages)

	if selectedMbox.NumMessages > 0 {
		seqSet := imap.SeqSetNum(1)                              // 设置序列号
		fetchOptions := &imap.FetchOptions{Envelope: true}       // 获取邮件信封信息
		messages, err := c.Fetch(seqSet, fetchOptions).Collect() // 获取邮件
		if err != nil {
			log.Fatalf("获取 INBOX 中第一封邮件失败: %v", err)
		}
		log.Printf("INBOX 中第一封邮件的主题: %v", messages[0].Envelope.Subject)
	}

	if err := c.Logout().Wait(); err != nil { // 登出
		log.Fatalf("登出失败: %v", err)
	}
}

// ExampleClient_pipelining 展示如何使用管道机制进行登录、选择和获取邮件。
func ExampleClient_pipelining() {
	var c *imapclient.Client

	uid := imap.UID(42)                                // 邮件 UID
	fetchOptions := &imap.FetchOptions{Envelope: true} // 获取邮件信封信息

	// 登录、选择和获取一封邮件在一个回合中完成
	loginCmd := c.Login("root", "root")                    // 登录
	selectCmd := c.Select("INBOX", nil)                    // 选择收件箱
	fetchCmd := c.Fetch(imap.UIDSetNum(uid), fetchOptions) // 获取邮件

	if err := loginCmd.Wait(); err != nil { // 等待登录完成
		log.Fatalf("登录失败: %v", err)
	}
	if _, err := selectCmd.Wait(); err != nil { // 等待选择完成
		log.Fatalf("选择 INBOX 失败: %v", err)
	}
	if messages, err := fetchCmd.Collect(); err != nil { // 等待获取邮件完成
		log.Fatalf("获取邮件失败: %v", err)
	} else {
		log.Printf("主题: %v", messages[0].Envelope.Subject) // 输出邮件主题
	}
}

// ExampleClient_Append 展示如何将邮件追加到 INBOX。
func ExampleClient_Append() {
	var c *imapclient.Client

	buf := []byte("From: <root@nsa.gov>\r\n\r\nHi <3") // 邮件内容
	size := int64(len(buf))                            // 邮件大小
	appendCmd := c.Append("INBOX", size, nil)          // 追加邮件
	if _, err := appendCmd.Write(buf); err != nil {    // 写入邮件内容
		log.Fatalf("写入邮件失败: %v", err)
	}
	if err := appendCmd.Close(); err != nil { // 关闭追加命令
		log.Fatalf("关闭邮件失败: %v", err)
	}
	if _, err := appendCmd.Wait(); err != nil { // 等待追加完成
		log.Fatalf("APPEND 命令失败: %v", err)
	}
}

// ExampleClient_Status 展示如何获取邮箱状态。
func ExampleClient_Status() {
	var c *imapclient.Client

	options := imap.StatusOptions{NumMessages: true} // 请求邮件数量
	if data, err := c.Status("INBOX", &options).Wait(); err != nil {
		log.Fatalf("STATUS 命令失败: %v", err)
	} else {
		log.Printf("INBOX 中包含 %v 封邮件", *data.NumMessages) // 输出邮件数量
	}
}

// ExampleClient_List_stream 展示如何使用流式方式列出邮箱并返回状态。
func ExampleClient_List_stream() {
	var c *imapclient.Client

	// ReturnStatus 要求服务器支持 IMAP4rev2 或 LIST-STATUS
	listCmd := c.List("", "%", &imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
		},
	})
	for {
		mbox := listCmd.Next() // 获取下一个邮箱
		if mbox == nil {
			break // 没有更多邮箱
		}
		log.Printf("邮箱 %q 包含 %v 封邮件 (%v 封未读)", mbox.Mailbox, mbox.Status.NumMessages, mbox.Status.NumUnseen) // 输出邮箱状态
	}
	if err := listCmd.Close(); err != nil { // 关闭命令
		log.Fatalf("LIST 命令失败: %v", err)
	}
}

// ExampleClient_Store 展示如何存储邮件标志。
func ExampleClient_Store() {
	var c *imapclient.Client

	seqSet := imap.SeqSetNum(1) // 设置序列号
	storeFlags := imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,            // 操作：添加标志
		Flags:  []imap.Flag{imap.FlagFlagged}, // 标志集合
		Silent: true,                          // 安静模式
	}
	if err := c.Store(seqSet, &storeFlags, nil).Close(); err != nil { // 存储标志并关闭
		log.Fatalf("STORE 命令失败: %v", err)
	}
}

// ExampleClient_Fetch 展示如何获取邮件。
func ExampleClient_Fetch() {
	var c *imapclient.Client

	seqSet := imap.SeqSetNum(1) // 设置序列号
	fetchOptions := &imap.FetchOptions{
		Flags:    true, // 获取标志
		Envelope: true, // 获取信封
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierHeader}, // 获取头部部分
		},
	}
	messages, err := c.Fetch(seqSet, fetchOptions).Collect() // 获取邮件
	if err != nil {
		log.Fatalf("FETCH 命令失败: %v", err)
	}

	msg := messages[0]
	var header []byte
	for _, buf := range msg.BodySection {
		header = buf // 获取邮件头部
		break
	}

	log.Printf("标志: %v", msg.Flags)            // 输出邮件标志
	log.Printf("主题: %v", msg.Envelope.Subject) // 输出邮件主题
	log.Printf("头部:\n%v", string(header))      // 输出邮件头部
}

// ExampleClient_Fetch_streamBody 展示如何使用流式方式获取邮件正文。
func ExampleClient_Fetch_streamBody() {
	var c *imapclient.Client

	seqSet := imap.SeqSetNum(1) // 设置序列号
	fetchOptions := &imap.FetchOptions{
		UID:         true,                             // 获取 UID
		BodySection: []*imap.FetchItemBodySection{{}}, // 获取邮件正文部分
	}
	fetchCmd := c.Fetch(seqSet, fetchOptions) // 获取邮件
	defer fetchCmd.Close()                    // 关闭命令

	for {
		msg := fetchCmd.Next() // 获取下一封邮件
		if msg == nil {
			break // 没有更多邮件
		}

		for {
			item := msg.Next() // 获取下一个项
			if item == nil {
				break // 没有更多项
			}

			switch item := item.(type) {
			case imapclient.FetchItemDataUID:
				log.Printf("UID: %v", item.UID) // 输出 UID
			case imapclient.FetchItemDataBodySection:
				b, err := io.ReadAll(item.Literal) // 读取正文
				if err != nil {
					log.Fatalf("读取正文部分失败: %v", err)
				}
				log.Printf("正文:\n%v", string(b)) // 输出正文
			}
		}
	}

	if err := fetchCmd.Close(); err != nil { // 关闭命令
		log.Fatalf("FETCH 命令失败: %v", err)
	}
}

// ExampleClient_Fetch_parseBody 展示如何解析邮件正文。
func ExampleClient_Fetch_parseBody() {
	var c *imapclient.Client

	// 发送 FETCH 命令以获取邮件正文
	seqSet := imap.SeqSetNum(1) // 设置序列号
	fetchOptions := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}}, // 获取邮件正文部分
	}
	fetchCmd := c.Fetch(seqSet, fetchOptions) // 获取邮件
	defer fetchCmd.Close()                    // 关闭命令

	msg := fetchCmd.Next() // 获取下一封邮件
	if msg == nil {
		log.Fatalf("FETCH 命令未返回任何邮件")
	}

	// 查找响应中的正文部分
	var bodySection imapclient.FetchItemDataBodySection
	ok := false
	for {
		item := msg.Next() // 获取下一个项
		if item == nil {
			break // 没有更多项
		}
		bodySection, ok = item.(imapclient.FetchItemDataBodySection) // 解析正文部分
		if ok {
			break
		}
	}
	if !ok {
		log.Fatalf("FETCH 命令未返回正文部分")
	}

	// 使用 go-message 库读取邮件
	mr, err := mail.CreateReader(bodySection.Literal)
	if err != nil {
		log.Fatalf("创建邮件阅读器失败: %v", err)
	}

	// 输出一些头部字段
	h := mr.Header
	if date, err := h.Date(); err != nil {
		log.Printf("解析日期头部字段失败: %v", err)
	} else {
		log.Printf("日期: %v", date) // 输出日期
	}
	if to, err := h.AddressList("To"); err != nil {
		log.Printf("解析收件人头部字段失败: %v", err)
	} else {
		log.Printf("收件人: %v", to) // 输出收件人
	}
	if subject, err := h.Text("Subject"); err != nil {
		log.Printf("解析主题头部字段失败: %v", err)
	} else {
		log.Printf("主题: %v", subject) // 输出主题
	}

	// 处理邮件的各个部分
	for {
		p, err := mr.NextPart() // 获取下一部分
		if err == io.EOF {
			break // 没有更多部分
		} else if err != nil {
			log.Fatalf("读取邮件部分失败: %v", err)
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			// 这是邮件的文本部分（可以是纯文本或 HTML）
			b, _ := io.ReadAll(p.Body)        // 读取正文
			log.Printf("内联文本: %v", string(b)) // 输出内联文本
		case *mail.AttachmentHeader:
			// 这是一个附件
			filename, _ := h.Filename()    // 获取附件名称
			log.Printf("附件: %v", filename) // 输出附件名称
		}
	}

	if err := fetchCmd.Close(); err != nil { // 关闭命令
		log.Fatalf("FETCH 命令失败: %v", err)
	}
}

// ExampleClient_Search 展示如何搜索邮件。
func ExampleClient_Search() {
	var c *imapclient.Client

	data, err := c.UIDSearch(&imap.SearchCriteria{
		Body: []string{"Hello world"}, // 根据邮件内容搜索
	}, nil).Wait() // 等待搜索结果
	if err != nil {
		log.Fatalf("UID SEARCH 命令失败: %v", err)
	}
	log.Fatalf("匹配搜索条件的 UID: %v", data.AllUIDs()) // 输出匹配的 UID
}

// ExampleClient_Idle 展示如何使用 IDLE 命令等待服务器更新。
func ExampleClient_Idle() {
	options := imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(seqNum uint32) {
				log.Printf("邮件 %v 已被删除", seqNum) // 输出删除的邮件序列号
			},
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					log.Printf("收到新邮件") // 输出新邮件通知
				}
			},
		},
	}

	c, err := imapclient.DialTLS("mail.example.org:993", &options) // 连接到 IMAP 服务器
	if err != nil {
		log.Fatalf("无法连接到 IMAP 服务器: %v", err)
	}
	defer c.Close() // 确保关闭连接

	if err := c.Login("root", "asdf").Wait(); err != nil { // 登录
		log.Fatalf("登录失败: %v", err)
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil { // 选择收件箱
		log.Fatalf("选择 INBOX 失败: %v", err)
	}

	// 开始等待更新
	idleCmd, err := c.Idle()
	if err != nil {
		log.Fatalf("IDLE 命令失败: %v", err)
	}

	// 等待 30 秒以接收来自服务器的更新
	time.Sleep(30 * time.Second)

	// 停止等待
	if err := idleCmd.Close(); err != nil {
		log.Fatalf("停止等待失败: %v", err)
	}
}

// ExampleClient_Authenticate_oauth 展示如何使用 OAuth 进行身份验证。
func ExampleClient_Authenticate_oauth() {
	var (
		c        *imapclient.Client
		username string
		token    string
	)

	if !c.Caps().Has(imap.AuthCap(sasl.OAuthBearer)) { // 检查服务器是否支持 OAUTHBEARER
		log.Fatal("服务器不支持 OAUTHBEARER")
	}

	saslClient := sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
		Username: username, // 用户名
		Token:    token,    // OAuth 令牌
	})
	if err := c.Authenticate(saslClient); err != nil { // 身份验证
		log.Fatalf("身份验证失败: %v", err)
	}
}
