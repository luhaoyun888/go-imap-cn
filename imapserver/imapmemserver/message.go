package imapmemserver

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"
)

// message 表示一封邮件的结构体。
// 包含不可变的 UID 和时间戳，以及可变的标志，标志由 Mailbox.mutex 保护。
type message struct {
	uid imap.UID  // 邮件的唯一标识符
	buf []byte    // 邮件内容的字节切片
	t   time.Time // 邮件的时间戳

	flags map[imap.Flag]struct{} // 邮件标志的集合
}

// fetch 方法用于提取邮件的相关信息。
// 参数：
//   - w: 用于写入提取结果的 FetchResponseWriter。
//   - options: 选择要提取的信息的选项。
//
// 返回：
//   - 返回错误信息（如果有）。
func (msg *message) fetch(w *imapserver.FetchResponseWriter, options *imap.FetchOptions) error {
	w.WriteUID(msg.uid) // 写入邮件的 UID

	if options.Flags {
		w.WriteFlags(msg.flagList()) // 写入邮件标志
	}
	if options.InternalDate {
		w.WriteInternalDate(msg.t) // 写入内部日期
	}
	if options.RFC822Size {
		w.WriteRFC822Size(int64(len(msg.buf))) // 写入 RFC822 大小
	}
	if options.Envelope {
		w.WriteEnvelope(msg.envelope()) // 写入信封信息
	}
	if bs := options.BodyStructure; bs != nil {
		w.WriteBodyStructure(msg.bodyStructure(bs.Extended)) // 写入邮件体结构
	}

	// 写入邮件的各个部分
	for _, bs := range options.BodySection {
		buf := msg.bodySection(bs)                    // 获取邮件部分内容
		wc := w.WriteBodySection(bs, int64(len(buf))) // 写入邮件部分
		_, writeErr := wc.Write(buf)                  // 写入内容
		closeErr := wc.Close()                        // 关闭写入器
		if writeErr != nil {
			return writeErr // 返回写入错误
		}
		if closeErr != nil {
			return closeErr // 返回关闭错误
		}
	}

	// TODO: BinarySection, BinarySectionSize

	return w.Close() // 关闭响应写入器
}

// envelope 方法用于获取邮件的信封信息。
// 返回：
//   - 返回 IMAP Envelope 结构体指针（如果解析成功）或 nil。
func (msg *message) envelope() *imap.Envelope {
	br := bufio.NewReader(bytes.NewReader(msg.buf)) // 创建字节读取器
	header, err := textproto.ReadHeader(br)         // 读取邮件头
	if err != nil {
		return nil // 返回 nil 表示失败
	}
	return getEnvelope(header) // 获取信封信息
}

// bodyStructure 方法用于获取邮件的体结构。
// 参数：
//   - extended: 是否获取扩展信息。
//
// 返回：
//   - 返回 IMAP BodyStructure 结构体。
func (msg *message) bodyStructure(extended bool) imap.BodyStructure {
	br := bufio.NewReader(bytes.NewReader(msg.buf)) // 创建字节读取器
	header, _ := textproto.ReadHeader(br)           // 读取邮件头
	return getBodyStructure(header, br, extended)   // 获取邮件体结构
}

// openMessagePart 方法用于打开邮件的部分内容。
// 参数：
//   - header: 邮件头。
//   - body: 邮件内容的读取器。
//   - parentMediaType: 父级媒体类型。
//
// 返回：
//   - 返回打开后的邮件头和读取器。
func openMessagePart(header textproto.Header, body io.Reader, parentMediaType string) (textproto.Header, io.Reader) {
	msgHeader := gomessage.Header{header}      // 创建 gomessage.Header
	mediaType, _, _ := msgHeader.ContentType() // 获取内容类型
	if !msgHeader.Has("Content-Type") && parentMediaType == "multipart/digest" {
		mediaType = "message/rfc822" // 如果没有内容类型并且是 multipart/digest，则设置为 message/rfc822
	}
	if mediaType == "message/rfc822" || mediaType == "message/global" {
		br := bufio.NewReader(body)          // 创建字节读取器
		header, _ = textproto.ReadHeader(br) // 读取头部
		return header, br                    // 返回头部和读取器
	}
	return header, body // 返回头部和原始内容
}

// bodySection 方法用于提取邮件的特定部分内容。
// 参数：
//   - item: 提取项，包含部分信息。
//
// 返回：
//   - 返回特定部分的字节切片（如果找到）或 nil。
func (msg *message) bodySection(item *imap.FetchItemBodySection) []byte {
	var (
		header textproto.Header
		body   io.Reader
	)

	br := bufio.NewReader(bytes.NewReader(msg.buf)) // 创建字节读取器
	header, err := textproto.ReadHeader(br)         // 读取邮件头
	if err != nil {
		return nil // 返回 nil 表示失败
	}
	body = br // 设置邮件内容读取器

	// 非 multipart 邮件的第一部分引用邮件本身
	msgHeader := gomessage.Header{header}      // 创建 gomessage.Header
	mediaType, _, _ := msgHeader.ContentType() // 获取内容类型
	partPath := item.Part                      // 获取部分路径
	if !strings.HasPrefix(mediaType, "multipart/") && len(partPath) > 0 && partPath[0] == 1 {
		partPath = partPath[1:] // 去掉前缀
	}

	// 使用提供的路径查找请求的部分
	var parentMediaType string
	for i := 0; i < len(partPath); i++ {
		partNum := partPath[i] // 当前部分编号

		header, body = openMessagePart(header, body, parentMediaType) // 打开当前部分
		msgHeader := gomessage.Header{header}                         // 创建 gomessage.Header
		mediaType, typeParams, _ := msgHeader.ContentType()           // 获取内容类型和参数
		if !strings.HasPrefix(mediaType, "multipart/") {
			if partNum != 1 {
				return nil // 如果不是第一部分，返回 nil
			}
			continue // 如果是第一部分，继续
		}

		mr := textproto.NewMultipartReader(body, typeParams["boundary"]) // 创建多部分读取器
		found := false
		for j := 1; j <= partNum; j++ {
			p, err := mr.NextPart() // 获取下一个部分
			if err != nil {
				return nil // 返回 nil 表示失败
			}

			if j == partNum { // 如果当前是目标部分
				parentMediaType = mediaType // 设置父级媒体类型
				header = p.Header           // 更新头部
				body = p                    // 更新内容读取器
				found = true
				break
			}
		}
		if !found {
			return nil // 如果未找到，返回 nil
		}
	}

	if len(item.Part) > 0 {
		switch item.Specifier {
		case imap.PartSpecifierHeader, imap.PartSpecifierText:
			header, body = openMessagePart(header, body, parentMediaType) // 打开当前部分
		}
	}

	// 过滤头部字段
	if len(item.HeaderFields) > 0 {
		keep := make(map[string]struct{}) // 保留字段的集合
		for _, k := range item.HeaderFields {
			keep[strings.ToLower(k)] = struct{}{} // 添加字段到保留集合
		}
		for field := header.Fields(); field.Next(); {
			if _, ok := keep[strings.ToLower(field.Key())]; !ok {
				field.Del() // 删除不在保留集合中的字段
			}
		}
	}
	for _, k := range item.HeaderFieldsNot {
		header.Del(k) // 删除不需要的字段
	}

	// 将请求的数据写入缓冲区
	var buf bytes.Buffer // 创建缓冲区
	writeHeader := true
	switch item.Specifier {
	case imap.PartSpecifierNone:
		writeHeader = len(item.Part) == 0 // 如果没有指定部分，写入头部
	case imap.PartSpecifierText:
		writeHeader = false // 如果是文本部分，不写入头部
	}
	if writeHeader {
		if err := textproto.WriteHeader(&buf, header); err != nil {
			return nil // 返回 nil 表示失败
		}
	}

	switch item.Specifier {
	case imap.PartSpecifierNone, imap.PartSpecifierText:
		if _, err := io.Copy(&buf, body); err != nil {
			return nil // 返回 nil 表示失败
		}
	}

	// 提取部分内容（如果有）
	b := buf.Bytes()
	if partial := item.Partial; partial != nil {
		end := partial.Offset + partial.Size // 计算结束位置
		if partial.Offset > int64(len(b)) {
			return nil // 如果偏移量超出范围，返回 nil
		}
		if end > int64(len(b)) {
			end = int64(len(b)) // 调整结束位置
		}
		b = b[partial.Offset:end] // 截取部分内容
	}
	return b // 返回提取的部分
}

// flagList 方法用于获取邮件标志的列表。
// 返回：
//   - 返回邮件标志的切片。
func (msg *message) flagList() []imap.Flag {
	var flags []imap.Flag
	for flag := range msg.flags {
		flags = append(flags, flag) // 将标志添加到切片
	}
	return flags // 返回标志切片
}

// store 方法用于存储邮件标志。
// 参数：
//   - store: 存储标志的操作结构体。
//
// 返回：
//   - 无
func (msg *message) store(store *imap.StoreFlags) {
	switch store.Op {
	case imap.StoreFlagsSet:
		msg.flags = make(map[imap.Flag]struct{}) // 设置新的标志集合
		fallthrough
	case imap.StoreFlagsAdd:
		for _, flag := range store.Flags {
			msg.flags[canonicalFlag(flag)] = struct{}{} // 添加标志
		}
	case imap.StoreFlagsDel:
		for _, flag := range store.Flags {
			delete(msg.flags, canonicalFlag(flag)) // 删除标志
		}
	default:
		panic(fmt.Errorf("未知的 STORE 标志操作: %v", store.Op)) // 抛出未知操作的错误
	}
}

// search 方法用于根据给定的搜索标准检查邮件。
// 参数：
//   - seqNum: 邮件的序列号。
//   - criteria: 搜索标准。
//
// 返回：
//   - 返回 true 表示邮件匹配搜索条件，false 表示不匹配。
func (msg *message) search(seqNum uint32, criteria *imap.SearchCriteria) bool {
	for _, seqSet := range criteria.SeqNum {
		if seqNum == 0 || !seqSet.Contains(seqNum) {
			return false // 如果序列号不匹配，返回 false
		}
	}
	for _, uidSet := range criteria.UID {
		if !uidSet.Contains(msg.uid) {
			return false // 如果 UID 不匹配，返回 false
		}
	}
	if !matchDate(msg.t, criteria.Since, criteria.Before) {
		return false // 如果日期不匹配，返回 false
	}

	for _, flag := range criteria.Flag {
		if _, ok := msg.flags[canonicalFlag(flag)]; !ok {
			return false // 如果标志不匹配，返回 false
		}
	}
	for _, flag := range criteria.NotFlag {
		if _, ok := msg.flags[canonicalFlag(flag)]; ok {
			return false // 如果不应有的标志存在，返回 false
		}
	}

	if criteria.Larger != 0 && int64(len(msg.buf)) <= criteria.Larger {
		return false // 如果邮件大小不符合要求，返回 false
	}
	if criteria.Smaller != 0 && int64(len(msg.buf)) >= criteria.Smaller {
		return false // 如果邮件大小不符合要求，返回 false
	}

	if !matchBytes(msg.buf, criteria.Text) {
		return false // 如果内容不匹配，返回 false
	}

	br := bufio.NewReader(bytes.NewReader(msg.buf))    // 创建字节读取器
	rawHeader, _ := textproto.ReadHeader(br)           // 读取邮件头
	header := mail.Header{gomessage.Header{rawHeader}} // 创建邮件头

	for _, fieldCriteria := range criteria.Header {
		if !header.Has(fieldCriteria.Key) {
			return false // 如果头部缺少必要字段，返回 false
		}
		if fieldCriteria.Value == "" {
			continue // 如果没有字段值，继续
		}
		found := false
		for _, v := range header.Values(fieldCriteria.Key) {
			found = strings.Contains(strings.ToLower(v), strings.ToLower(fieldCriteria.Value)) // 检查字段值是否匹配
			if found {
				break
			}
		}
		if !found {
			return false // 如果未找到匹配的值，返回 false
		}
	}

	if !criteria.SentSince.IsZero() || !criteria.SentBefore.IsZero() {
		t, err := header.Date() // 获取邮件发送日期
		if err != nil {
			return false // 如果获取失败，返回 false
		} else if !matchDate(t, criteria.SentSince, criteria.SentBefore) {
			return false // 如果发送日期不符合要求，返回 false
		}
	}

	if len(criteria.Body) > 0 {
		body, _ := io.ReadAll(br) // 读取邮件正文
		if !matchBytes(body, criteria.Body) {
			return false // 如果正文不匹配，返回 false
		}
	}

	for _, not := range criteria.Not {
		if msg.search(seqNum, &not) {
			return false // 如果不应存在的条件匹配，返回 false
		}
	}
	for _, or := range criteria.Or {
		if !msg.search(seqNum, &or[0]) && !msg.search(seqNum, &or[1]) {
			return false // 如果或条件都不匹配，返回 false
		}
	}

	return true // 返回 true 表示匹配
}

// matchDate 方法用于比较日期是否在给定的范围内。
// 参数：
//   - t: 要比较的日期。
//   - since: 开始日期。
//   - before: 结束日期。
//
// 返回：
//   - 返回 true 表示在范围内，false 表示不在范围内。
func matchDate(t, since, before time.Time) bool {
	// 我们通过将时区信息设置为 UTC 来丢弃时区信息。
	// RFC 3501 明确要求无时区的日期比较。
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

	if !since.IsZero() && t.Before(since) {
		return false // 如果早于开始日期，返回 false
	}
	if !before.IsZero() && !t.Before(before) {
		return false // 如果不早于结束日期，返回 false
	}
	return true // 返回 true 表示在范围内
}

// matchBytes 方法用于检查字节数组是否包含给定的模式。
// 参数：
//   - buf: 要检查的字节数组。
//   - patterns: 要匹配的模式。
//
// 返回：
//   - 返回 true 表示匹配，false 表示不匹配。
func matchBytes(buf []byte, patterns []string) bool {
	if len(patterns) == 0 {
		return true // 如果没有模式，返回 true
	}
	buf = bytes.ToLower(buf) // 转为小写进行匹配
	for _, s := range patterns {
		if !bytes.Contains(buf, bytes.ToLower([]byte(s))) {
			return false // 如果不包含模式，返回 false
		}
	}
	return true // 返回 true 表示匹配
}

// getEnvelope 方法用于从头部信息中获取邮件信封。
// 参数：
//   - h: 邮件头部。
//
// 返回：
//   - 返回 IMAP Envelope 结构体指针。
func getEnvelope(h textproto.Header) *imap.Envelope {
	mh := mail.Header{gomessage.Header{h}}      // 创建邮件头
	date, _ := mh.Date()                        // 获取日期
	inReplyTo, _ := mh.MsgIDList("In-Reply-To") // 获取回复消息 ID
	messageID, _ := mh.MessageID()              // 获取消息 ID
	return &imap.Envelope{                      // 返回信封信息
		Date:      date,
		Subject:   h.Get("Subject"),
		From:      parseAddressList(mh, "From"),
		Sender:    parseAddressList(mh, "Sender"),
		ReplyTo:   parseAddressList(mh, "Reply-To"),
		To:        parseAddressList(mh, "To"),
		Cc:        parseAddressList(mh, "Cc"),
		Bcc:       parseAddressList(mh, "Bcc"),
		InReplyTo: inReplyTo,
		MessageID: messageID,
	}
}

// parseAddressList 方法用于解析邮件地址列表。
// 参数：
//   - mh: 邮件头。
//   - k: 要解析的字段名。
//
// 返回：
//   - 返回解析后的 IMAP Address 列表。
func parseAddressList(mh mail.Header, k string) []imap.Address {
	// TODO: 保持引号词不变
	// TODO: 处理组地址
	addrs, _ := mh.AddressList(k) // 获取地址列表
	var l []imap.Address
	for _, addr := range addrs {
		mailbox, host, ok := strings.Cut(addr.Address, "@") // 分割地址
		if !ok {
			continue // 如果格式不正确，继续
		}
		l = append(l, imap.Address{ // 添加到地址列表
			Name:    mime.QEncoding.Encode("utf-8", addr.Name), // 编码名称
			Mailbox: mailbox,
			Host:    host,
		})
	}
	return l // 返回地址列表
}

// canonicalFlag 方法用于返回规范化的邮件标志。
// 参数：
//   - flag: 邮件标志。
//
// 返回：
//   - 返回小写的邮件标志。
func canonicalFlag(flag imap.Flag) imap.Flag {
	return imap.Flag(strings.ToLower(string(flag))) // 转为小写
}

// getBodyStructure 方法用于获取邮件的正文结构。
// 参数：
//   - rawHeader: 原始邮件头。
//   - r: 邮件正文读取器。
//   - extended: 是否返回扩展信息。
//
// 返回：
//   - 返回 IMAP BodyStructure 结构体。
func getBodyStructure(rawHeader textproto.Header, r io.Reader, extended bool) imap.BodyStructure {
	header := gomessage.Header{rawHeader} // 创建邮件头

	mediaType, typeParams, _ := header.ContentType()       // 获取媒体类型和参数
	primaryType, subType, _ := strings.Cut(mediaType, "/") // 分割媒体类型

	if primaryType == "multipart" {
		bs := &imap.BodyStructureMultiPart{Subtype: subType}          // 创建多部分结构
		mr := textproto.NewMultipartReader(r, typeParams["boundary"]) // 创建多部分读取器
		for {
			part, _ := mr.NextPart() // 获取下一个部分
			if part == nil {
				break // 如果没有更多部分，退出循环
			}
			bs.Children = append(bs.Children, getBodyStructure(part.Header, part, extended)) // 添加部分结构
		}
		if extended {
			bs.Extended = &imap.BodyStructureMultiPartExt{ // 如果需要扩展信息，添加
				Params:      typeParams,
				Disposition: getContentDisposition(header),
				Language:    getContentLanguage(header),
				Location:    header.Get("Content-Location"),
			}
		}
		return bs // 返回多部分结构
	} else {
		body, _ := io.ReadAll(r) // 读取正文内容
		bs := &imap.BodyStructureSinglePart{
			Type:        primaryType,
			Subtype:     subType,
			Params:      typeParams,
			ID:          header.Get("Content-Id"),
			Description: header.Get("Content-Description"),
			Encoding:    header.Get("Content-Transfer-Encoding"),
			Size:        uint32(len(body)), // 计算正文大小
		}
		if mediaType == "message/rfc822" || mediaType == "message/global" {
			br := bufio.NewReader(bytes.NewReader(body)) // 创建字节读取器
			childHeader, _ := textproto.ReadHeader(br)   // 读取子邮件头
			bs.MessageRFC822 = &imap.BodyStructureMessageRFC822{
				Envelope:      getEnvelope(childHeader),                    // 获取信封
				BodyStructure: getBodyStructure(childHeader, br, extended), // 获取子邮件结构
				NumLines:      int64(bytes.Count(body, []byte("\n"))),      // 计算行数
			}
		}
		if primaryType == "text" {
			bs.Text = &imap.BodyStructureText{
				NumLines: int64(bytes.Count(body, []byte("\n"))), // 计算行数
			}
		}
		if extended {
			bs.Extended = &imap.BodyStructureSinglePartExt{
				Disposition: getContentDisposition(header),
				Language:    getContentLanguage(header),
				Location:    header.Get("Content-Location"),
			}
		}
		return bs // 返回单部分结构
	}
}

// getContentDisposition 方法用于获取内容处置信息。
// 参数：
//   - header: 邮件头。
//
// 返回：
//   - 返回 IMAP BodyStructureDisposition 结构体指针。
func getContentDisposition(header gomessage.Header) *imap.BodyStructureDisposition {
	disp, dispParams, _ := header.ContentDisposition() // 获取处置类型和参数
	if disp == "" {
		return nil // 如果没有处置信息，返回 nil
	}
	return &imap.BodyStructureDisposition{
		Value:  disp,
		Params: dispParams,
	}
}

// getContentLanguage 方法用于获取内容语言信息。
// 参数：
//   - header: 邮件头。
//
// 返回：
//   - 返回语言列表。
func getContentLanguage(header gomessage.Header) []string {
	v := header.Get("Content-Language") // 获取内容语言字段
	if v == "" {
		return nil // 如果没有语言信息，返回 nil
	}
	// TODO: 处理 CFWS
	l := strings.Split(v, ",") // 分割语言列表
	for i, lang := range l {
		l[i] = strings.TrimSpace(lang) // 去除多余空格
	}
	return l // 返回语言列表
}
