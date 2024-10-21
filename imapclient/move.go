package imapclient

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// Move 发送 MOVE 命令。
//
// 如果服务器不支持 IMAP4rev2 或 MOVE 扩展，则使用 COPY + STORE + EXPUNGE 命令作为回退方案。
func (c *Client) Move(numSet imap.NumSet, mailbox string) *MoveCommand {
	// 如果服务器不支持 MOVE，则回退到 [UID] COPY，
	// [UID] STORE +FLAGS.SILENT \Deleted 和 [UID] EXPUNGE
	cmdName := "MOVE"
	if !c.Caps().Has(imap.CapMove) {
		cmdName = "COPY" // 选择使用 COPY 命令
	}

	cmd := &MoveCommand{}
	enc := c.beginCommand(uidCmdName(cmdName, imapwire.NumSetKind(numSet)), cmd)
	enc.SP().NumSet(numSet).SP().Mailbox(mailbox) // 设置命令参数
	enc.end()

	// 如果使用 COPY 命令，则设置相应的 STORE 和 EXPUNGE 命令
	if cmdName == "COPY" {
		cmd.store = c.Store(numSet, &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagDeleted}, // 标记为删除
		}, nil)
		if uidSet, ok := numSet.(imap.UIDSet); ok && c.Caps().Has(imap.CapUIDPlus) {
			cmd.expunge = c.UIDExpunge(uidSet) // 使用 UIDExpunge
		} else {
			cmd.expunge = c.Expunge() // 使用普通的 Expunge
		}
	}

	return cmd
}

// MoveCommand 是 MOVE 命令的结构体。
type MoveCommand struct {
	commandBase
	data MoveData

	// 回退命令
	store   *FetchCommand
	expunge *ExpungeCommand
}

// Wait 等待 MOVE 命令完成，并返回结果数据。
func (cmd *MoveCommand) Wait() (*MoveData, error) {
	if err := cmd.wait(); err != nil {
		return nil, err
	}
	if cmd.store != nil {
		if err := cmd.store.Close(); err != nil {
			return nil, err
		}
	}
	if cmd.expunge != nil {
		if err := cmd.expunge.Close(); err != nil {
			return nil, err
		}
	}
	return &cmd.data, nil
}

// MoveData 包含 MOVE 命令返回的数据。
type MoveData struct {
	// 需要 UIDPLUS 或 IMAP4rev2
	UIDValidity uint32      // UID 有效性
	SourceUIDs  imap.NumSet // 源 UID 集合
	DestUIDs    imap.NumSet // 目标 UID 集合
}
