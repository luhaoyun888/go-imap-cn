package imapclient

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// SortKey 表示排序关键字
type SortKey string

const (
	SortKeyArrival SortKey = "ARRIVAL" // 按到达时间排序
	SortKeyCc      SortKey = "CC"      // 按抄送人排序
	SortKeyDate    SortKey = "DATE"    // 按日期排序
	SortKeyFrom    SortKey = "FROM"    // 按发件人排序
	SortKeySize    SortKey = "SIZE"    // 按大小排序
	SortKeySubject SortKey = "SUBJECT" // 按主题排序
	SortKeyTo      SortKey = "TO"      // 按收件人排序
)

// SortCriterion 表示排序标准
type SortCriterion struct {
	Key     SortKey // 排序关键字
	Reverse bool    // 是否反向排序
}

// SortOptions 包含 SORT 命令的选项。
type SortOptions struct {
	SearchCriteria *imap.SearchCriteria // 搜索条件
	SortCriteria   []SortCriterion      // 排序条件
}

// sort 发送一个 SORT 命令。
func (c *Client) sort(numKind imapwire.NumKind, options *SortOptions) *SortCommand {
	cmd := &SortCommand{}                                   // 创建一个新的 SORT 命令
	enc := c.beginCommand(uidCmdName("SORT", numKind), cmd) // 开始发送 SORT 命令
	enc.SP().List(len(options.SortCriteria), func(i int) {
		criterion := options.SortCriteria[i]
		if criterion.Reverse {
			enc.Atom("REVERSE").SP() // 如果是反向排序，添加 REVERSE
		}
		enc.Atom(string(criterion.Key)) // 添加排序关键字
	})
	enc.SP().Atom("UTF-8").SP()                         // 设置字符编码为 UTF-8
	writeSearchKey(enc.Encoder, options.SearchCriteria) // 写入搜索条件
	enc.end()                                           // 结束命令
	return cmd                                          // 返回命令
}

// handleSort 处理 SORT 命令的响应。
func (c *Client) handleSort() error {
	cmd := findPendingCmdByType[*SortCommand](c) // 查找待处理的 SORT 命令
	for c.dec.SP() {
		var num uint32
		if !c.dec.ExpectNumber(&num) {
			return c.dec.Err() // 返回错误
		}
		if cmd != nil {
			cmd.nums = append(cmd.nums, num) // 将数字添加到命令中
		}
	}
	return nil
}

// Sort 发送一个 SORT 命令。
//
// 此命令需要支持 SORT 扩展。
func (c *Client) Sort(options *SortOptions) *SortCommand {
	return c.sort(imapwire.NumKindSeq, options) // 调用 sort 方法
}

// UIDSort 发送一个 UID SORT 命令。
//
// 参见 Sort。
func (c *Client) UIDSort(options *SortOptions) *SortCommand {
	return c.sort(imapwire.NumKindUID, options) // 调用 sort 方法
}

// SortCommand 是一个 SORT 命令。
type SortCommand struct {
	commandBase
	nums []uint32 // 排序结果的序号
}

// Wait 等待 SORT 命令完成，并返回结果。
func (cmd *SortCommand) Wait() ([]uint32, error) {
	err := cmd.wait()    // 等待命令完成
	return cmd.nums, err // 返回结果
}
