package imapclient

import (
	"fmt"

	"github.com/emersion/go-sasl"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
)

// Authenticate 发送 AUTHENTICATE 命令。
//
// 与其他命令不同，此方法会阻塞，直到 SASL 交换完成。
func (c *Client) Authenticate(saslClient sasl.Client) error {
	mech, initialResp, err := saslClient.Start() // 启动 SASL 认证
	if err != nil {
		return err
	}

	// c.Caps 可能会发送 CAPABILITY 命令，因此在 c.beginCommand 之前检查
	var hasSASLIR bool
	if initialResp != nil {
		hasSASLIR = c.Caps().Has(imap.CapSASLIR)
	}

	cmd := &authenticateCommand{}
	contReq := c.registerContReq(cmd)          // 注册继续请求
	enc := c.beginCommand("AUTHENTICATE", cmd) // 开始 AUTHENTICATE 命令
	enc.SP().Atom(mech)                        // 设置认证机制
	if initialResp != nil && hasSASLIR {
		enc.SP().Atom(internal.EncodeSASL(initialResp)) // 添加初始响应
		initialResp = nil
	}
	enc.flush()     // 刷新编码
	defer enc.end() // 结束命令

	for {
		challengeStr, err := contReq.Wait() // 等待挑战字符串
		if err != nil {
			return cmd.wait() // 等待命令响应
		}

		if challengeStr == "" {
			if initialResp == nil {
				return fmt.Errorf("imapclient: 服务器请求 SASL 初始响应，但我们没有")
			}

			contReq = c.registerContReq(cmd) // 重新注册继续请求
			if err := c.writeSASLResp(initialResp); err != nil {
				return err // 写入 SASL 响应时出错
			}
			initialResp = nil
			continue
		}

		challenge, err := internal.DecodeSASL(challengeStr) // 解码挑战
		if err != nil {
			return err
		}

		resp, err := saslClient.Next(challenge) // 获取下一个 SASL 响应
		if err != nil {
			return err
		}

		contReq = c.registerContReq(cmd) // 重新注册继续请求
		if err := c.writeSASLResp(resp); err != nil {
			return err // 写入 SASL 响应时出错
		}
	}
}

type authenticateCommand struct {
	commandBase
}

// writeSASLResp 写入 SASL 响应。
func (c *Client) writeSASLResp(resp []byte) error {
	respStr := internal.EncodeSASL(resp) // 编码 SASL 响应
	if _, err := c.bw.WriteString(respStr + "\r\n"); err != nil {
		return err // 写入时出错
	}
	if err := c.bw.Flush(); err != nil {
		return err // 刷新时出错
	}
	return nil
}

// Unauthenticate 发送 UNAUTHENTICATE 命令。
//
// 此命令需要支持 UNAUTHENTICATE 扩展。
func (c *Client) Unauthenticate() *Command {
	cmd := &unauthenticateCommand{}
	c.beginCommand("UNAUTHENTICATE", cmd).end() // 开始 UNAUTHENTICATE 命令
	return &cmd.Command
}

type unauthenticateCommand struct {
	Command
}
