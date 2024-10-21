package imapclient_test

import (
	"crypto/tls"
	"io"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/imapclient"
	"github.com/luhaoyun888/go-imap-cn/imapserver"
	"github.com/luhaoyun888/go-imap-cn/imapserver/imapmemserver"
)

const (
	testUsername = "test-user"
	testPassword = "test-password"
)

const simpleRawMessage = `MIME-Version: 1.0
Message-Id: <191101702316132@example.com>
Content-Transfer-Encoding: 8bit
Content-Type: text/plain; charset=utf-8

这是我的信！`

var rsaCertPEM = `-----BEGIN CERTIFICATE-----
MIIDOTCCAiGgAwIBAgIQSRJrEpBGFc7tNb1fb5pKFzANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA6Gba5tHV1dAKouAaXO3/ebDUU4rvwCUg/CNaJ2PT5xLD4N1Vcb8r
bFSW2HXKq+MPfVdwIKR/1DczEoAGf/JWQTW7EgzlXrCd3rlajEX2D73faWJekD0U
aUgz5vtrTXZ90BQL7WvRICd7FlEZ6FPOcPlumiyNmzUqtwGhO+9ad1W5BqJaRI6P
YfouNkwR6Na4TzSj5BrqUfP0FwDizKSJ0XXmh8g8G9mtwxOSN3Ru1QFc61Xyeluk
POGKBV/q6RBNklTNe0gI8usUMlYyoC7ytppNMW7X2vodAelSu25jgx2anj9fDVZu
h7AXF5+4nJS4AAt0n1lNY7nGSsdZas8PbQIDAQABo4GIMIGFMA4GA1UdDwEB/wQE
AwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1Ud
DgQWBBStsdjh3/JCXXYlQryOrL4Sh7BW5TAuBgNVHREEJzAlggtleGFtcGxlLmNv
bYcEfwAAAYcQAAAAAAAAAAAAAAAAAAAAATANBgkqhkiG9w0BAQsFAAOCAQEAxWGI
5NhpF3nwwy/4yB4i/CwwSpLrWUa70NyhvprUBC50PxiXav1TeDzwzLx/o5HyNwsv
cxv3HdkLW59i/0SlJSrNnWdfZ19oTcS+6PtLoVyISgtyN6DpkKpdG1cOkW3Cy2P2
+tK/tKHRP1Y/Ra0RiDpOAmqn0gCOFGz8+lqDIor/T7MTpibL3IxqWfPrvfVRHL3B
grw/ZQTTIVjjh4JBSW3WyWgNo/ikC1lrVxzl4iPUGptxT36Cr7Zk2Bsg0XqwbOvK
5d+NTDREkSnUbie4GeutujmX3Dsx88UiV6UY/4lHJa6I5leHUNOHahRbpbWeOfs/
WkBKOclmOV2xlTVuPw==
-----END CERTIFICATE-----
`

var rsaKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDoZtrm0dXV0Aqi
4Bpc7f95sNRTiu/AJSD8I1onY9PnEsPg3VVxvytsVJbYdcqr4w99V3AgpH/UNzMS
gAZ/8lZBNbsSDOVesJ3euVqMRfYPvd9pYl6QPRRpSDPm+2tNdn3QFAvta9EgJ3sW
URnoU85w+W6aLI2bNSq3AaE771p3VbkGolpEjo9h+i42TBHo1rhPNKPkGupR8/QX
AOLMpInRdeaHyDwb2a3DE5I3dG7VAVzrVfJ6W6Q84YoFX+rpEE2SVM17SAjy6xQy
VjKgLvK2mk0xbtfa+h0B6VK7bmODHZqeP18NVm6HsBcXn7iclLgAC3SfWU1jucZK
x1lqzw9tAgMBAAECggEABWzxS1Y2wckblnXY57Z+sl6YdmLV+gxj2r8Qib7g4ZIk
lIlWR1OJNfw7kU4eryib4fc6nOh6O4AWZyYqAK6tqNQSS/eVG0LQTLTTEldHyVJL
dvBe+MsUQOj4nTndZW+QvFzbcm2D8lY5n2nBSxU5ypVoKZ1EqQzytFcLZpTN7d89
EPj0qDyrV4NZlWAwL1AygCwnlwhMQjXEalVF1ylXwU3QzyZ/6MgvF6d3SSUlh+sq
XefuyigXw484cQQgbzopv6niMOmGP3of+yV4JQqUSb3IDmmT68XjGd2Dkxl4iPki
6ZwXf3CCi+c+i/zVEcufgZ3SLf8D99kUGE7v7fZ6AQKBgQD1ZX3RAla9hIhxCf+O
3D+I1j2LMrdjAh0ZKKqwMR4JnHX3mjQI6LwqIctPWTU8wYFECSh9klEclSdCa64s
uI/GNpcqPXejd0cAAdqHEEeG5sHMDt0oFSurL4lyud0GtZvwlzLuwEweuDtvT9cJ
Wfvl86uyO36IW8JdvUprYDctrQKBgQDycZ697qutBieZlGkHpnYWUAeImVA878sJ
w44NuXHvMxBPz+lbJGAg8Cn8fcxNAPqHIraK+kx3po8cZGQywKHUWsxi23ozHoxo
+bGqeQb9U661TnfdDspIXia+xilZt3mm5BPzOUuRqlh4Y9SOBpSWRmEhyw76w4ZP
OPxjWYAgwQKBgA/FehSYxeJgRjSdo+MWnK66tjHgDJE8bYpUZsP0JC4R9DL5oiaA
brd2fI6Y+SbyeNBallObt8LSgzdtnEAbjIH8uDJqyOmknNePRvAvR6mP4xyuR+Bv
m+Lgp0DMWTw5J9CKpydZDItc49T/mJ5tPhdFVd+am0NAQnmr1MCZ6nHxAoGABS3Y
LkaC9FdFUUqSU8+Chkd/YbOkuyiENdkvl6t2e52jo5DVc1T7mLiIrRQi4SI8N9bN
/3oJWCT+uaSLX2ouCtNFunblzWHBrhxnZzTeqVq4SLc8aESAnbslKL4i8/+vYZlN
s8xtiNcSvL+lMsOBORSXzpj/4Ot8WwTkn1qyGgECgYBKNTypzAHeLE6yVadFp3nQ
Ckq9yzvP/ib05rvgbvrne00YeOxqJ9gtTrzgh7koqJyX1L4NwdkEza4ilDWpucn0
xiUZS4SoaJq6ZvcBYS62Yr1t8n09iG47YL8ibgtmH3L+svaotvpVxVK+d7BLevA/
ZboOWVe3icTy64BT3OQhmg==
-----END RSA PRIVATE KEY-----
`

// newMemClientServerPair 创建一个内存客户端和服务器的配对，用于测试。
// 参数：
//
//	t - 测试对象，用于报告测试失败。
//
// 返回值：
//
//	net.Conn - 与服务器的连接。
//	io.Closer - 用于关闭服务器的接口。
func newMemClientServerPair(t *testing.T) (net.Conn, io.Closer) {
	memServer := imapmemserver.New() // 创建一个内存 IMAP 服务器

	user := imapmemserver.NewUser(testUsername, testPassword) // 创建用户
	user.Create("INBOX", nil)                                 // 创建 INBOX 文件夹

	memServer.AddUser(user) // 将用户添加到服务器

	cert, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM)) // 生成 TLS 证书
	if err != nil {
		t.Fatalf("tls.X509KeyPair() = %v", err) // 如果出错，报告失败
	}

	server := imapserver.New(&imapserver.Options{ // 创建新的 IMAP 服务器实例
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.NewSession(), nil, nil // 创建新的会话
		},
		TLSConfig: &tls.Config{ // 配置 TLS
			Certificates: []tls.Certificate{cert},
		},
		InsecureAuth: true, // 允许不安全的身份验证
		Caps: imap.CapSet{ // 设置服务器功能
			imap.CapIMAP4rev1: {},
			imap.CapIMAP4rev2: {},
		},
	})

	ln, err := net.Listen("tcp", "localhost:0") // 监听本地端口
	if err != nil {
		t.Fatalf("net.Listen() = %v", err) // 如果出错，报告失败
	}

	go func() {
		if err := server.Serve(ln); err != nil { // 启动服务器
			t.Errorf("Serve() = %v", err)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String()) // 与服务器建立连接
	if err != nil {
		t.Fatalf("net.Dial() = %v", err) // 如果出错，报告失败
	}

	return conn, server // 返回连接和服务器
}

// newClientServerPair 创建一个客户端和服务器的配对。
// 参数：
//
//	t - 测试对象，用于报告测试失败。
//	initialState - 初始连接状态。
//
// 返回值：
//
//	*imapclient.Client - IMAP 客户端。
//	io.Closer - 用于关闭服务器的接口。
func newClientServerPair(t *testing.T, initialState imap.ConnState) (*imapclient.Client, io.Closer) {
	var useDovecot bool
	switch os.Getenv("GOIMAP_TEST_DOVECOT") {
	case "0", "":
		// ok
	case "1":
		useDovecot = true
	default:
		t.Fatalf("无效的 GOIMAP_TEST_DOVECOT 环境变量") // 报告无效的环境变量
	}

	var (
		conn   net.Conn
		server io.Closer
	)
	if useDovecot {
		if initialState < imap.ConnStateAuthenticated {
			t.Skip("Dovecot 连接是预先认证的") // 跳过不符合状态的测试
		}
		conn, server = newDovecotClientServerPair(t) // 创建 Dovecot 客户端和服务器配对
	} else {
		conn, server = newMemClientServerPair(t) // 创建内存客户端和服务器配对
	}

	var debugWriter swapWriter
	debugWriter.Swap(io.Discard) // 设置调试写入器

	var options imapclient.Options
	if testing.Verbose() {
		options.DebugWriter = &debugWriter // 如果是详细模式，则启用调试输出
	}
	client := imapclient.New(conn, &options) // 创建新的 IMAP 客户端

	if initialState >= imap.ConnStateAuthenticated {
		// Dovecot 连接是预先认证的
		if !useDovecot {
			if err := client.Login(testUsername, testPassword).Wait(); err != nil {
				t.Fatalf("Login().Wait() = %v", err) // 登录失败，报告错误
			}
		}

		appendCmd := client.Append("INBOX", int64(len(simpleRawMessage)), nil) // 附加消息到 INBOX
		appendCmd.Write([]byte(simpleRawMessage))                              // 写入消息
		appendCmd.Close()                                                      // 关闭附加命令
		if _, err := appendCmd.Wait(); err != nil {
			t.Fatalf("AppendCommand.Wait() = %v", err) // 等待附加命令失败，报告错误
		}
	}
	if initialState >= imap.ConnStateSelected {
		if _, err := client.Select("INBOX", nil).Wait(); err != nil {
			t.Fatalf("Select().Wait() = %v", err) // 选择 INBOX 失败，报告错误
		}
	}

	// 在初始化测试完成后启用调试日志
	debugWriter.Swap(os.Stderr)

	return client, server // 返回客户端和服务器
}

// swapWriter 是一个可以在运行时交换的 io.Writer。
type swapWriter struct {
	w     io.Writer  // 当前写入器
	mutex sync.Mutex // 互斥锁
}

// Write 写入数据到当前写入器。
// 参数：
//
//	b - 要写入的字节切片。
//
// 返回值：
//
//	int - 写入的字节数。
//	error - 写入过程中的错误。
func (sw *swapWriter) Write(b []byte) (int, error) {
	sw.mutex.Lock() // 加锁以防止并发写入
	w := sw.w
	sw.mutex.Unlock() // 解锁

	return w.Write(b) // 写入数据
}

// Swap 交换当前写入器。
// 参数：
//
//	w - 新的 io.Writer。
func (sw *swapWriter) Swap(w io.Writer) {
	sw.mutex.Lock()   // 加锁
	sw.w = w          // 交换写入器
	sw.mutex.Unlock() // 解锁
}

// TestLogin 测试登录功能。
func TestLogin(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateNotAuthenticated) // 创建客户端和服务器配对
	defer client.Close()                                                     // 确保在测试结束时关闭客户端
	defer server.Close()                                                     // 确保在测试结束时关闭服务器

	if err := client.Login(testUsername, testPassword).Wait(); err != nil {
		t.Errorf("Login().Wait() = %v", err) // 登录失败，报告错误
	}
}

// TestLogout 测试注销功能。
func TestLogout(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateAuthenticated) // 创建客户端和服务器配对
	defer server.Close()                                                  // 确保在测试结束时关闭服务器

	if _, ok := server.(*dovecotServer); ok {
		t.Skip("Dovecot 连接不会回复 LOGOUT") // 跳过 Dovecot 测试
	}

	if err := client.Logout().Wait(); err != nil {
		t.Errorf("Logout().Wait() = %v", err) // 注销失败，报告错误
	}
	if err := client.Close(); err != nil {
		t.Errorf("Close() = %v", err) // 关闭客户端失败，报告错误
	}
}

// https://github.com/emersion/go-imap/issues/562
// TestFetch_invalid 测试无效的获取请求。
func TestFetch_invalid(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected) // 创建客户端和服务器配对
	defer client.Close()                                             // 确保在测试结束时关闭客户端
	defer server.Close()                                             // 确保在测试结束时关闭服务器

	_, err := client.Fetch(imap.UIDSet(nil), nil).Collect() // 尝试收集无效的获取请求
	if err == nil {
		t.Fatalf("UIDFetch().Collect() = %v", err) // 如果没有错误，则报告测试失败
	}
}

// TestFetch_closeUnreadBody 测试关闭未读邮件主体的获取。
func TestFetch_closeUnreadBody(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected) // 创建客户端和服务器配对
	defer client.Close()                                             // 确保在测试结束时关闭客户端
	defer server.Close()                                             // 确保在测试结束时关闭服务器

	fetchCmd := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{ // 创建获取命令
		BodySection: []*imap.FetchItemBodySection{ // 设置获取选项
			{
				Specifier: imap.PartSpecifierNone,
				Peek:      true,
			},
		},
	})
	if err := fetchCmd.Close(); err != nil {
		t.Fatalf("UIDFetch().Close() = %v", err) // 关闭获取命令失败，报告错误
	}
}

// TestWaitGreeting_eof 测试等待问候消息时的 EOF 情况。
func TestWaitGreeting_eof(t *testing.T) {
	// 不良服务器：已连接但没有问候消息
	clientConn, serverConn := net.Pipe() // 创建客户端和服务器管道

	client := imapclient.New(clientConn, nil) // 创建新的 IMAP 客户端
	defer client.Close()                      // 确保在测试结束时关闭客户端

	if err := serverConn.Close(); err != nil {
		t.Fatalf("serverConn.Close() = %v", err) // 关闭服务器连接失败，报告错误
	}

	if err := client.WaitGreeting(); err == nil {
		t.Fatalf("WaitGreeting() 应该失败") // 如果没有错误，则报告测试失败
	}
}
