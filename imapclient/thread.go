package imapclient

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// ThreadOptions 包含 THREAD 命令的选项
type ThreadOptions struct {
	Algorithm      imap.ThreadAlgorithm // 线程算法
	SearchCriteria *imap.SearchCriteria // 搜索条件
}

// thread 方法，发送 THREAD 命令
// numKind: 数字种类（序列或 UID）
// options: 线程选项
// 返回值: 返回一个 ThreadCommand 结构体指针
func (c *Client) thread(numKind imapwire.NumKind, options *ThreadOptions) *ThreadCommand {
	cmd := &ThreadCommand{}
	enc := c.beginCommand(uidCmdName("THREAD", numKind), cmd)
	enc.SP().Atom(string(options.Algorithm)).SP().Atom("UTF-8").SP()
	writeSearchKey(enc.Encoder, options.SearchCriteria) // 写入搜索关键字
	enc.end()
	return cmd
}

// Thread 方法，发送 THREAD 命令
// 该命令需要支持 THREAD 扩展
func (c *Client) Thread(options *ThreadOptions) *ThreadCommand {
	return c.thread(imapwire.NumKindSeq, options)
}

// UIDThread 方法，发送 UID THREAD 命令
// 参见 Thread 方法
func (c *Client) UIDThread(options *ThreadOptions) *ThreadCommand {
	return c.thread(imapwire.NumKindUID, options)
}

// handleThread 方法，处理 THREAD 响应
func (c *Client) handleThread() error {
	cmd := findPendingCmdByType[*ThreadCommand](c)
	for c.dec.SP() {
		data, err := readThreadList(c.dec) // 读取线程列表
		if err != nil {
			return fmt.Errorf("在线程列表中: %v", err)
		}
		if cmd != nil {
			cmd.data = append(cmd.data, *data)
		}
	}
	return nil
}

// ThreadCommand 是一个 THREAD 命令的结构体
type ThreadCommand struct {
	commandBase
	data []ThreadData // 线程数据
}

// Wait 方法等待命令完成并返回线程数据
func (cmd *ThreadCommand) Wait() ([]ThreadData, error) {
	err := cmd.wait()
	return cmd.data, err
}

// ThreadData 表示线程数据
type ThreadData struct {
	Chain      []uint32     // 线程链
	SubThreads []ThreadData // 子线程
}

// readThreadList 方法，读取线程列表
// dec: 解码器
// 返回值: 返回一个 ThreadData 结构体指针和可能的错误
func readThreadList(dec *imapwire.Decoder) (*ThreadData, error) {
	var data ThreadData
	err := dec.ExpectList(func() error {
		var num uint32
		if len(data.SubThreads) == 0 && dec.Number(&num) {
			data.Chain = append(data.Chain, num) // 添加到链中
		} else {
			sub, err := readThreadList(dec) // 递归读取子线程
			if err != nil {
				return err
			}
			data.SubThreads = append(data.SubThreads, *sub) // 添加子线程
		}
		return nil
	})
	return &data, err
}
