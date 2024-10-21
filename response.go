package imap

import (
	"fmt"
	"strings"
)

// StatusResponseType 是一种通用状态响应类型。
type StatusResponseType string

const (
	StatusResponseTypeOK      StatusResponseType = "OK"      // 表示请求成功
	StatusResponseTypeNo      StatusResponseType = "NO"      // 表示请求失败
	StatusResponseTypeBad     StatusResponseType = "BAD"     // 表示请求无效
	StatusResponseTypePreAuth StatusResponseType = "PREAUTH" // 表示已预先授权
	StatusResponseTypeBye     StatusResponseType = "BYE"     // 表示会话结束
)

// ResponseCode 是一种响应代码。
type ResponseCode string

const (
	ResponseCodeAlert                ResponseCode = "ALERT"                // 警告
	ResponseCodeAlreadyExists        ResponseCode = "ALREADYEXISTS"        // 已存在
	ResponseCodeAuthenticationFailed ResponseCode = "AUTHENTICATIONFAILED" // 身份验证失败
	ResponseCodeAuthorizationFailed  ResponseCode = "AUTHORIZATIONFAILED"  // 授权失败
	ResponseCodeBadCharset           ResponseCode = "BADCHARSET"           // 字符集错误
	ResponseCodeCannot               ResponseCode = "CANNOT"               // 无法执行
	ResponseCodeClientBug            ResponseCode = "CLIENTBUG"            // 客户端错误
	ResponseCodeContactAdmin         ResponseCode = "CONTACTADMIN"         // 联系管理员
	ResponseCodeCorruption           ResponseCode = "CORRUPTION"           // 数据损坏
	ResponseCodeExpired              ResponseCode = "EXPIRED"              // 过期
	ResponseCodeHasChildren          ResponseCode = "HASCHILDREN"          // 有子项
	ResponseCodeInUse                ResponseCode = "INUSE"                // 正在使用
	ResponseCodeLimit                ResponseCode = "LIMIT"                // 限制
	ResponseCodeNonExistent          ResponseCode = "NONEXISTENT"          // 不存在
	ResponseCodeNoPerm               ResponseCode = "NOPERM"               // 无权限
	ResponseCodeOverQuota            ResponseCode = "OVERQUOTA"            // 超出配额
	ResponseCodeParse                ResponseCode = "PARSE"                // 解析错误
	ResponseCodePrivacyRequired      ResponseCode = "PRIVACYREQUIRED"      // 需要隐私
	ResponseCodeServerBug            ResponseCode = "SERVERBUG"            // 服务器错误
	ResponseCodeTryCreate            ResponseCode = "TRYCREATE"            // 尝试创建
	ResponseCodeUnavailable          ResponseCode = "UNAVAILABLE"          // 不可用
	ResponseCodeUnknownCTE           ResponseCode = "UNKNOWN-CTE"          // 未知内容传输编码

	// METADATA
	ResponseCodeTooMany   ResponseCode = "TOOMANY"   // 太多
	ResponseCodeNoPrivate ResponseCode = "NOPRIVATE" // 无法访问私人元数据

	// APPENDLIMIT
	ResponseCodeTooBig ResponseCode = "TOOBIG" // 太大
)

// StatusResponse 是一种通用状态响应。
//
// 参见 RFC 9051 第 7.1 节。
type StatusResponse struct {
	Type StatusResponseType // 状态响应类型
	Code ResponseCode       // 响应代码
	Text string             // 额外信息
}

// Error 是由状态响应引起的 IMAP 错误。
type Error StatusResponse

var _ error = (*Error)(nil)

// Error 实现了 error 接口。
func (err *Error) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "imap: %v", err.Type) // 输出状态类型
	if err.Code != "" {
		fmt.Fprintf(&sb, " [%v]", err.Code) // 输出响应代码
	}
	text := err.Text
	if text == "" {
		text = "<unknown>" // 如果文本为空，设置为未知
	}
	fmt.Fprintf(&sb, " %v", text) // 输出额外信息
	return sb.String()
}
