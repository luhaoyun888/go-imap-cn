package imapclient

import (
	"fmt"
	"sync/atomic"
	"time"
)

const idleRestartInterval = 28 * time.Minute // IDLE 命令重启间隔

// Idle 发送 IDLE 命令。
//
// 与其他命令不同，此方法会阻塞，直到服务器确认该命令。
// 成功后，IDLE 命令将运行，其他命令无法发送。
// 调用者必须调用 IdleCommand.Close 来停止 IDLE 并解除客户端的阻塞。
//
// 此命令要求支持 IMAP4rev2 或 IDLE 扩展。IDLE
// 命令会自动重启，以避免因不活动超时而断开连接。
func (c *Client) Idle() (*IdleCommand, error) {
	child, err := c.idle() // 发送 IDLE 命令
	if err != nil {
		return nil, err
	}

	cmd := &IdleCommand{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go cmd.run(c, child) // 启动 IDLE 命令的运行
	return cmd, nil
}

// IdleCommand 表示 IDLE 命令。
//
// 最初，IDLE 命令正在运行。服务器可能会发送单方面的数据。
// 在 IDLE 运行期间，客户端无法发送任何命令。
//
// 必须调用 Close 来停止 IDLE 命令。
type IdleCommand struct {
	stopped atomic.Bool
	stop    chan struct{}
	done    chan struct{}

	err       error
	lastChild *idleCommand
}

// run 运行 IDLE 命令。
func (cmd *IdleCommand) run(c *Client, child *idleCommand) {
	defer close(cmd.done) // 关闭完成通道

	timer := time.NewTimer(idleRestartInterval) // 创建重启定时器
	defer timer.Stop()

	defer func() {
		if child != nil {
			if err := child.Close(); err != nil && cmd.err == nil {
				cmd.err = err // 记录关闭错误
			}
		}
	}()

	for {
		select {
		case <-timer.C: // 如果定时器到期
			timer.Reset(idleRestartInterval) // 重置定时器

			if cmd.err = child.Close(); cmd.err != nil {
				return // 关闭子命令出错
			}
			if child, cmd.err = c.idle(); cmd.err != nil {
				return // 发送新的 IDLE 命令出错
			}
		case <-c.decCh: // 如果接收到解码通道数据
			cmd.lastChild = child
			return
		case <-cmd.stop: // 如果收到停止信号
			cmd.lastChild = child
			return
		}
	}
}

// Close 停止 IDLE 命令。
//
// 此方法会阻塞，直到停止 IDLE 的命令被写入，但不等待服务器的响应。
// 调用者可以使用 Wait 来等待服务器响应。
func (cmd *IdleCommand) Close() error {
	if cmd.stopped.Swap(true) {
		return fmt.Errorf("imapclient: IDLE 已经关闭")
	}
	close(cmd.stop) // 发送停止信号
	<-cmd.done      // 等待完成
	return cmd.err  // 返回错误
}

// Wait 阻塞直到 IDLE 命令完成。
func (cmd *IdleCommand) Wait() error {
	<-cmd.done
	if cmd.err != nil {
		return cmd.err // 返回错误
	}
	return cmd.lastChild.Wait() // 等待最后一个子命令完成
}

// idle 发送 IDLE 命令并返回命令句柄。
func (c *Client) idle() (*idleCommand, error) {
	cmd := &idleCommand{}
	contReq := c.registerContReq(cmd)     // 注册连续请求
	cmd.enc = c.beginCommand("IDLE", cmd) // 开始 IDLE 命令
	cmd.enc.flush()                       // 刷新编码器

	_, err := contReq.Wait() // 等待连续请求完成
	if err != nil {
		cmd.enc.end() // 结束编码
		return nil, err
	}

	return cmd, nil
}

// idleCommand 表示一个单独的 IDLE 命令，没有重启逻辑。
type idleCommand struct {
	commandBase
	enc *commandEncoder // 编码器
}

// Close 停止 IDLE 命令。
//
// 此方法会阻塞，直到停止 IDLE 的命令被写入，但不等待服务器的响应。
// 调用者可以使用 Wait 来等待服务器响应。
func (cmd *idleCommand) Close() error {
	if cmd.err != nil {
		return cmd.err // 如果已有错误，返回错误
	}
	if cmd.enc == nil {
		return fmt.Errorf("imapclient: IDLE 命令被关闭两次")
	}
	cmd.enc.client.setWriteTimeout(cmdWriteTimeout)     // 设置写入超时
	_, err := cmd.enc.client.bw.WriteString("DONE\r\n") // 发送 DONE 命令
	if err == nil {
		err = cmd.enc.client.bw.Flush() // 刷新缓冲区
	}
	cmd.enc.end() // 结束编码
	cmd.enc = nil // 清空编码器
	return err
}

// Wait 阻塞直到 IDLE 命令完成。
//
// Wait 只能在 Close 之后调用。
func (cmd *idleCommand) Wait() error {
	if cmd.enc != nil {
		panic("imapclient: idleCommand.Close 必须在 Wait 之前调用")
	}
	return cmd.wait() // 等待命令完成
}
