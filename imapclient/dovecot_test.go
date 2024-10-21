package imapclient_test

import (
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newDovecotClientServerPair 创建一个 Dovecot 客户端和服务器对。
// 返回值：
//
//	net.Conn - 客户端连接。
//	io.Closer - 用于关闭服务器的接口。
func newDovecotClientServerPair(t *testing.T) (net.Conn, io.Closer) {
	tempDir := t.TempDir() // 创建临时目录

	cfgFilename := filepath.Join(tempDir, "dovecot.conf") // 配置文件路径
	cfg := `log_path      = "` + tempDir + `/dovecot.log"
ssl           = no
mail_home     = "` + tempDir + `/%u"
mail_location = maildir:~/Mail

namespace inbox {
	separator = /
	prefix =
	inbox = yes
}

mail_plugins = $mail_plugins acl
protocol imap {
	mail_plugins = $mail_plugins imap_acl
}
plugin {
  acl = vfile
}
` // Dovecot 配置内容
	if err := os.WriteFile(cfgFilename, []byte(cfg), 0666); err != nil { // 写入配置文件
		t.Fatalf("写入 Dovecot 配置失败: %v", err)
	}

	clientConn, serverConn := net.Pipe() // 创建客户端和服务器管道

	cmd := exec.Command("doveadm", "-c", cfgFilename, "exec", "imap")       // 创建执行 Dovecot 的命令
	cmd.Env = []string{"USER=" + testUsername, "PATH=" + os.Getenv("PATH")} // 设置环境变量
	cmd.Dir = tempDir                                                       // 设置工作目录
	cmd.Stdin = serverConn                                                  // 设置输入流
	cmd.Stdout = serverConn                                                 // 设置输出流
	cmd.Stderr = os.Stderr                                                  // 设置错误输出流
	if err := cmd.Start(); err != nil {                                     // 启动 Dovecot
		t.Fatalf("启动 Dovecot 失败: %v", err)
	}

	return clientConn, &dovecotServer{cmd, serverConn} // 返回客户端连接和服务器实例
}

// dovecotServer 是 Dovecot 服务器的结构体。
type dovecotServer struct {
	cmd  *exec.Cmd // 执行命令的结构体
	conn net.Conn  // 服务器连接
}

// Close 关闭 Dovecot 服务器。
// 返回值：
//
//	error - 关闭过程中的错误。
func (srv *dovecotServer) Close() error {
	if err := srv.conn.Close(); err != nil { // 关闭连接
		return err // 返回错误
	}
	return srv.cmd.Wait() // 等待命令结束
}
