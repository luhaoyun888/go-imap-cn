package internal

import (
	"github.com/luhaoyun888/go-imap-cn"
)

func FormatRights(rm imap.RightModification, rs imap.RightSet) string {
	s := ""
	if rm != imap.RightModificationReplace {
		s = string(rm)
	}
	return s + string(rs)
}
