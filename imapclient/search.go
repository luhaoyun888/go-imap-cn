package imapclient

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// 返回搜索选项的字符串列表
// options: 传入的搜索选项指针
// 返回值: 返回一个字符串切片，包含所有被选中的搜索选项
func returnSearchOptions(options *imap.SearchOptions) []string {
	if options == nil {
		return nil
	}

	m := map[string]bool{
		"MIN":   options.ReturnMin,   // 返回最小值
		"MAX":   options.ReturnMax,   // 返回最大值
		"ALL":   options.ReturnAll,   // 返回所有
		"COUNT": options.ReturnCount, // 返回计数
	}

	var l []string
	for k, ret := range m {
		if ret {
			l = append(l, k)
		}
	}
	return l
}

// 搜索方法，发送搜索命令
// numKind: 数字种类（序列或UID）
// criteria: 搜索条件
// options: 搜索选项
// 返回值: 返回一个SearchCommand结构体指针
func (c *Client) search(numKind imapwire.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) *SearchCommand {
	// IMAP4rev2的搜索字符集默认为UTF-8。当启用UTF8=ACCEPT时，指定任何CHARSET都是无效的。
	var charset string
	if !c.Caps().Has(imap.CapIMAP4rev2) && !c.enabled.Has(imap.CapUTF8Accept) && !searchCriteriaIsASCII(criteria) {
		charset = "UTF-8"
	}

	var all imap.NumSet
	switch numKind {
	case imapwire.NumKindSeq:
		all = imap.SeqSet(nil)
	case imapwire.NumKindUID:
		all = imap.UIDSet(nil)
	}

	cmd := &SearchCommand{}
	cmd.data.All = all
	enc := c.beginCommand(uidCmdName("SEARCH", numKind), cmd)
	if returnOpts := returnSearchOptions(options); len(returnOpts) > 0 {
		enc.SP().Atom("RETURN").SP().List(len(returnOpts), func(i int) {
			enc.Atom(returnOpts[i])
		})
	}
	enc.SP()
	if charset != "" {
		enc.Atom("CHARSET").SP().Atom(charset).SP()
	}
	writeSearchKey(enc.Encoder, criteria)
	enc.end()
	return cmd
}

// Search方法，发送一个SEARCH命令
// criteria: 搜索条件
// options: 搜索选项
// 返回值: 返回一个SearchCommand结构体指针
func (c *Client) Search(criteria *imap.SearchCriteria, options *imap.SearchOptions) *SearchCommand {
	return c.search(imapwire.NumKindSeq, criteria, options)
}

// UIDSearch方法，发送一个UID SEARCH命令
// criteria: 搜索条件
// options: 搜索选项
// 返回值: 返回一个SearchCommand结构体指针
func (c *Client) UIDSearch(criteria *imap.SearchCriteria, options *imap.SearchOptions) *SearchCommand {
	return c.search(imapwire.NumKindUID, criteria, options)
}

// 处理搜索响应
func (c *Client) handleSearch() error {
	cmd := findPendingCmdByType[*SearchCommand](c)
	for c.dec.SP() {
		if c.dec.Special('(') {
			var name string
			if !c.dec.ExpectAtom(&name) || !c.dec.ExpectSP() {
				return c.dec.Err()
			} else if strings.ToUpper(name) != "MODSEQ" {
				return fmt.Errorf("在搜索排序MODSEQ中：期望 %q，得到 %q", "MODSEQ", name)
			}
			var modSeq uint64
			if !c.dec.ExpectModSeq(&modSeq) || !c.dec.ExpectSpecial(')') {
				return c.dec.Err()
			}
			if cmd != nil {
				cmd.data.ModSeq = modSeq
			}
			break
		}

		var num uint32
		if !c.dec.ExpectNumber(&num) {
			return c.dec.Err()
		}
		if cmd != nil {
			switch all := cmd.data.All.(type) {
			case imap.SeqSet:
				all.AddNum(num)
				cmd.data.All = all
			case imap.UIDSet:
				all.AddNum(imap.UID(num))
				cmd.data.All = all
			}
		}
	}
	return nil
}

// 处理扩展搜索响应
func (c *Client) handleESearch() error {
	if !c.dec.ExpectSP() {
		return c.dec.Err()
	}
	tag, data, err := readESearchResponse(c.dec)
	if err != nil {
		return err
	}
	cmd := c.findPendingCmdFunc(func(anyCmd command) bool {
		cmd, ok := anyCmd.(*SearchCommand)
		if !ok {
			return false
		}
		if tag != "" {
			return cmd.tag == tag
		} else {
			return true
		}
	})
	if cmd != nil {
		cmd := cmd.(*SearchCommand)
		cmd.data = *data
	}
	return nil
}

// SearchCommand是一个搜索命令的结构体
type SearchCommand struct {
	commandBase
	data imap.SearchData // 搜索数据
}

// Wait方法等待命令完成并返回搜索数据
func (cmd *SearchCommand) Wait() (*imap.SearchData, error) {
	return &cmd.data, cmd.wait()
}

// 写入搜索关键字
// enc: 编码器
// criteria: 搜索条件
func writeSearchKey(enc *imapwire.Encoder, criteria *imap.SearchCriteria) {
	firstItem := true
	encodeItem := func() *imapwire.Encoder {
		if !firstItem {
			enc.SP()
		}
		firstItem = false
		return enc
	}

	for _, seqSet := range criteria.SeqNum {
		encodeItem().NumSet(seqSet)
	}
	for _, uidSet := range criteria.UID {
		encodeItem().Atom("UID").SP().NumSet(uidSet)
	}

	if !criteria.Since.IsZero() && !criteria.Before.IsZero() && criteria.Before.Sub(criteria.Since) == 24*time.Hour {
		encodeItem().Atom("ON").SP().String(criteria.Since.Format(internal.DateLayout))
	} else {
		if !criteria.Since.IsZero() {
			encodeItem().Atom("SINCE").SP().String(criteria.Since.Format(internal.DateLayout))
		}
		if !criteria.Before.IsZero() {
			encodeItem().Atom("BEFORE").SP().String(criteria.Before.Format(internal.DateLayout))
		}
	}
	if !criteria.SentSince.IsZero() && !criteria.SentBefore.IsZero() && criteria.SentBefore.Sub(criteria.SentSince) == 24*time.Hour {
		encodeItem().Atom("SENTON").SP().String(criteria.SentSince.Format(internal.DateLayout))
	} else {
		if !criteria.SentSince.IsZero() {
			encodeItem().Atom("SENTSINCE").SP().String(criteria.SentSince.Format(internal.DateLayout))
		}
		if !criteria.SentBefore.IsZero() {
			encodeItem().Atom("SENTBEFORE").SP().String(criteria.SentBefore.Format(internal.DateLayout))
		}
	}

	for _, kv := range criteria.Header {
		switch k := strings.ToUpper(kv.Key); k {
		case "BCC", "CC", "FROM", "SUBJECT", "TO":
			encodeItem().Atom(k)
		default:
			encodeItem().Atom("HEADER").SP().String(kv.Key)
		}
		enc.SP().String(kv.Value)
	}

	for _, s := range criteria.Body {
		encodeItem().Atom("BODY").SP().String(s)
	}
	for _, s := range criteria.Text {
		encodeItem().Atom("TEXT").SP().String(s)
	}

	for _, flag := range criteria.Flag {
		if k := flagSearchKey(flag); k != "" {
			encodeItem().Atom(k)
		} else {
			encodeItem().Atom("KEYWORD").SP().Flag(flag)
		}
	}
	for _, flag := range criteria.NotFlag {
		if k := flagSearchKey(flag); k != "" {
			encodeItem().Atom("UN" + k)
		} else {
			encodeItem().Atom("UNKEYWORD").SP().Flag(flag)
		}
	}

	if criteria.Larger > 0 {
		encodeItem().Atom("LARGER").SP().Number64(criteria.Larger)
	}
	if criteria.Smaller > 0 {
		encodeItem().Atom("SMALLER").SP().Number64(criteria.Smaller)
	}

	if modSeq := criteria.ModSeq; modSeq != nil {
		encodeItem().Atom("MODSEQ")
		if modSeq.MetadataName != "" && modSeq.MetadataType != "" {
			enc.SP().Quoted(modSeq.MetadataName).SP().Atom(string(modSeq.MetadataType))
		}
		enc.SP()
		if modSeq.ModSeq != 0 {
			enc.ModSeq(modSeq.ModSeq)
		} else {
			enc.Atom("0")
		}
	}

	for _, not := range criteria.Not {
		encodeItem().Atom("NOT").SP()
		enc.Special('(')
		writeSearchKey(enc, &not)
		enc.Special(')')
	}
	for _, or := range criteria.Or {
		encodeItem().Atom("OR").SP()
		enc.Special('(')
		writeSearchKey(enc, &or[0])
		enc.Special(')')
		enc.SP()
		enc.Special('(')
		writeSearchKey(enc, &or[1])
		enc.Special(')')
	}

	if firstItem {
		enc.Atom("所有") // "ALL" replaced with "所有"
	}
}

// 根据标志返回搜索关键字
// flag: IMAP标志
// 返回值: 返回对应的搜索关键字字符串
func flagSearchKey(flag imap.Flag) string {
	switch flag {
	case imap.FlagAnswered, imap.FlagDeleted, imap.FlagDraft, imap.FlagFlagged, imap.FlagSeen:
		return strings.ToUpper(strings.TrimPrefix(string(flag), "\\"))
	default:
		return ""
	}
}

// 读取扩展搜索响应
// dec: 解码器
// 返回值: 返回tag字符串、搜索数据结构体指针和可能的错误
func readESearchResponse(dec *imapwire.Decoder) (tag string, data *imap.SearchData, err error) {
	data = &imap.SearchData{}
	if dec.Special('(') { // 搜索相关器
		var correlator string
		if !dec.ExpectAtom(&correlator) || !dec.ExpectSP() || !dec.ExpectAString(&tag) || !dec.ExpectSpecial(')') {
			return "", nil, dec.Err()
		}
		if correlator != "TAG" {
			return "", nil, fmt.Errorf("在搜索相关器中：名称必须是TAG，但得到 %q", correlator)
		}
	}

	var name string
	if !dec.SP() {
		return tag, data, nil
	} else if !dec.ExpectAtom(&name) {
		return "", nil, dec.Err()
	}
	data.UID = name == "UID"

	if data.UID {
		if !dec.SP() {
			return tag, data, nil
		} else if !dec.ExpectAtom(&name) {
			return "", nil, dec.Err()
		}
	}

	for {
		if !dec.ExpectSP() {
			return "", nil, dec.Err()
		}

		switch strings.ToUpper(name) {
		case "MIN":
			var num uint32
			if !dec.ExpectNumber(&num) {
				return "", nil, dec.Err()
			}
			data.Min = num
		case "MAX":
			var num uint32
			if !dec.ExpectNumber(&num) {
				return "", nil, dec.Err()
			}
			data.Max = num
		case "ALL":
			numKind := imapwire.NumKindSeq
			if data.UID {
				numKind = imapwire.NumKindUID
			}
			if !dec.ExpectNumSet(numKind, &data.All) {
				return "", nil, dec.Err()
			}
			if data.All.Dynamic() {
				return "", nil, fmt.Errorf("imapclient: 服务器返回了一个动态的ALL数字集合，不能在SEARCH响应中使用")
			}
		case "COUNT":
			var num uint32
			if !dec.ExpectNumber(&num) {
				return "", nil, dec.Err()
			}
			data.Count = num
		case "MODSEQ":
			var modSeq uint64
			if !dec.ExpectModSeq(&modSeq) {
				return "", nil, dec.Err()
			}
			data.ModSeq = modSeq
		default:
			if !dec.DiscardValue() {
				return "", nil, dec.Err()
			}
		}

		if !dec.SP() {
			break
		} else if !dec.ExpectAtom(&name) {
			return "", nil, dec.Err()
		}
	}

	return tag, data, nil
}

// 判断搜索条件是否全部为ASCII字符
// criteria: 搜索条件
// 返回值: 返回布尔值，表示是否全部为ASCII
func searchCriteriaIsASCII(criteria *imap.SearchCriteria) bool {
	for _, kv := range criteria.Header {
		if !isASCII(kv.Key) || !isASCII(kv.Value) {
			return false
		}
	}
	for _, s := range criteria.Body {
		if !isASCII(s) {
			return false
		}
	}
	for _, s := range criteria.Text {
		if !isASCII(s) {
			return false
		}
	}
	for _, not := range criteria.Not {
		if !searchCriteriaIsASCII(&not) {
			return false
		}
	}
	for _, or := range criteria.Or {
		if !searchCriteriaIsASCII(&or[0]) || !searchCriteriaIsASCII(&or[1]) {
			return false
		}
	}
	return true
}

// 判断字符串是否为ASCII字符
// s: 待判断字符串
// 返回值: 返回布尔值，表示是否为ASCII字符
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}
