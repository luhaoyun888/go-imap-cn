// imapmemserver包实现了一个内存中的IMAP服务器。
package imapmemserver

import (
	"sync"

	"github.com/emersion/go-imap/v2/imapserver"
)

// Server 是一个服务器实例。
//
// 服务器包含用户列表。
type Server struct {
	mutex sync.Mutex       // 互斥锁，用于保护用户列表的并发访问
	users map[string]*User // 用户列表，以用户名为键，User 结构体指针为值
}

// New 创建一个新的服务器实例。
// 返回一个 Server 结构体指针。
func New() *Server {
	return &Server{
		users: make(map[string]*User), // 初始化用户列表
	}
}

// NewSession 创建一个新的 IMAP 会话。
// 返回一个实现了 imapserver.Session 接口的 serverSession 结构体指针。
func (s *Server) NewSession() imapserver.Session {
	return &serverSession{server: s} // 创建新的服务器会话
}

// user 是一个私有方法，用于根据用户名获取用户。
// 参数：
//   - username: 用户名。
//
// 返回：
//   - 返回 User 结构体指针（如果存在）或 nil。
func (s *Server) user(username string) *User {
	s.mutex.Lock()           // 锁定
	defer s.mutex.Unlock()   // 解锁
	return s.users[username] // 返回用户
}

// AddUser 将用户添加到服务器。
// 参数：
//   - user: 要添加的 User 结构体指针。
func (s *Server) AddUser(user *User) {
	s.mutex.Lock()                // 锁定
	s.users[user.username] = user // 添加用户
	s.mutex.Unlock()              // 解锁
}

// serverSession 是与特定服务器关联的会话。
// 可能包含 UserSession 指针，用户会话可以为 nil。
type serverSession struct {
	*UserSession         // 可能为 nil 的用户会话指针
	server       *Server // 不可变的服务器指针
}

var _ imapserver.Session = (*serverSession)(nil) // 确保 serverSession 实现了 Session 接口

// Login 方法用于用户登录。
// 参数：
//   - username: 用户名。
//   - password: 密码。
//
// 返回：
//   - 返回错误信息（如果有）。
func (sess *serverSession) Login(username, password string) error {
	u := sess.server.user(username) // 获取用户
	if u == nil {
		return imapserver.ErrAuthFailed // 如果用户不存在，返回认证失败错误
	}
	if err := u.Login(username, password); err != nil {
		return err // 如果登录失败，返回错误
	}
	sess.UserSession = NewUserSession(u) // 创建用户会话
	return nil                           // 返回 nil 表示成功
}
