package imapserver

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleIdle 处理 IDLE 命令的请求。
//
// dec: 用于解码的 imapwire.Decoder。
func (c *Conn) handleIdle(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 如果没有期望的 CRLF，返回解码错误
	}

	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 检查连接状态是否为已验证，若不是则返回错误
	}

	if err := c.writeContReq("正在等待"); err != nil { // 将 "idling" 替换为 "正在等待"
		return err // 发送 IDLE 请求的持续状态
	}

	stop := make(chan struct{}) // 创建停止信号通道
	done := make(chan error, 1) // 创建完成信号通道
	go func() {
		defer func() {
			if v := recover(); v != nil {
				c.server.logger().Printf("处理 IDLE 时发生恐慌: %v\n%s", v, debug.Stack()) // 记录发生的恐慌和堆栈信息
				done <- fmt.Errorf("imapserver: 处理 IDLE 时发生恐慌")                     // 发送恐慌错误到完成信号通道
			}
		}()
		w := &UpdateWriter{conn: c, allowExpunge: true} // 创建更新写入器
		done <- c.session.Idle(w, stop)                 // 进入 IDLE 状态并等待停止信号
	}()

	c.setReadTimeout(idleReadTimeout)      // 设置读取超时
	line, isPrefix, err := c.br.ReadLine() // 读取一行输入
	close(stop)                            // 关闭停止信号通道
	if err == io.EOF {
		return nil // 如果到达文件结束，返回 nil
	} else if err != nil {
		return err // 其他错误返回
	} else if isPrefix || string(line) != "完成" { // 将 "DONE" 替换为 "完成"
		return newClientBugError("语法错误: 期望以 '完成' 结束 IDLE 命令") // 处理语法错误
	}

	return <-done // 返回完成信号的结果
}
