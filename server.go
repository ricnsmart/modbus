package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type (
	Server struct {
		// Addr optionally specifies the TCP address for the server to listen on,
		// in the form "host:port".
		// The service names are defined in RFC 6335 and assigned by IANA.
		// See net.Dial for details of the address format.
		Addr string

		ReadTimeout time.Duration

		WriteTimeout time.Duration

		// 下行命令超时
		DownloadCmdTimeout time.Duration

		// 一次性读取字节流的最大长度
		MaxBytes int

		// 保存所有活动连接
		mu sync.Mutex

		activeConn map[*conn]struct{}

		inShutdown atomicBool // true when server is in shutdown

		// 处理设备上行报文
		uploadHandler func(remoteAddr string, in []byte) []byte

		// function to call after Shutdown
		afterShutdown func()

		// function to call after connection close
		afterConnClose func(remoteAddr string)

		logger Logger
	}

	// A conn represents the server side of an tcp connection.
	conn struct {
		// server is the server on which the connection arrived.
		// Immutable; never nil.
		server *Server

		// rwc is the underlying network connection.
		// This is never wrapped by other types and is the value given out
		// to CloseNotifier callers. It is usually of type *net.TCPConn or
		// *tls.conn.
		rwc net.Conn

		// remoteAddr is rwc.RemoteAddr().String()
		// 可以用来作为标识链接的唯一值
		remoteAddr string

		inShutdown atomicBool // true when conn is in shutdown

		inDownloading atomicBool // true when downloading command

		receiveCmdCh chan []byte // 接收命令

		sendCmdRespCh chan []byte // 发送命令响应

		errorCh chan error

		cancel context.CancelFunc
	}
)

func (s *Server) SetAfterConnClose(afterConnClose func(remoteAddr string)) {
	s.afterConnClose = afterConnClose
}

func (s *Server) SetAfterShutdown(afterShutdown func()) {
	s.afterShutdown = afterShutdown
}

type atomicBool int32

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

const (
	defaultMaxBytes        = 500
	defaultDownloadTimeout = 20 * time.Second
	defaultWriteTimeout    = 1 * time.Minute
	defaultReadTimeout     = 4 * time.Minute
	readFailedFormat       = "read failed: %v,conn: %v"
	writeFailedFormat      = "write failed: %v,conn: %v"
)

// ErrServerClosed is returned by the Server's Serve, ServeTLS, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("modbus: Server already closed")

var ErrConnectionClosed = errors.New("modbus: Connection already closed")

var ErrPacketTooLarge = errors.New("modbus: packet too large")

var ErrDeviceNotOnline = errors.New("modbus: device not online")

var ErrDownloadCmdTooBusy = errors.New("modbus: downloading command,try later")

var ErrDownloadCmdTimeout = errors.New("modbus: download command timeout")

var ErrReceiveCmdResponseTimeout = errors.New("modbus: receive command response timeout")

func NewServer() *Server {
	return &Server{
		logger:             NewDefaultLogger(),
		MaxBytes:           defaultMaxBytes,
		ReadTimeout:        defaultReadTimeout,
		WriteTimeout:       defaultWriteTimeout,
		DownloadCmdTimeout: defaultDownloadTimeout,
		activeConn:         make(map[*conn]struct{}, 0),
	}
}

func (s *Server) SetUploadHandler(uploadHandler func(remoteAddr string, in []byte) []byte) {
	s.uploadHandler = uploadHandler
}

func (s *Server) CountActiveConn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeConn)
}

func (s *Server) SetLogger(l Logger) {
	s.logger = l
}

func (s *Server) info(v ...interface{}) {
	s.logger.Info(v...)
}

func (s *Server) warn(v ...interface{}) {
	s.logger.Warn(v...)
}

func (s *Server) error(v ...interface{}) {
	s.logger.Error(v...)
}

func (s *Server) debug(v ...interface{}) {
	s.logger.Debug(v...)
}

func (s *Server) shuttingDown() bool {
	return s.inShutdown.isSet()
}

func (s *Server) StartServer(address string) error {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf(`modbus: failed to listen port %v , reason: %v`, address, err)
	}
	defer l.Close()

	var tempDelay time.Duration // how long to sleep on accept failure

	for {
		rwc, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				s.error("modbus: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}
		tempDelay = 0
		c := s.newConn(rwc)
		s.trackConn(c, true)
		go c.serve()
	}
}

func (s *Server) Shutdown() error {
	if s.shuttingDown() {
		return ErrServerClosed
	}
	s.debug("modbus: server shutting down")
	s.inShutdown.setTrue()
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		c.inShutdown.setTrue()
		// 不管连接是否在活动都予以关闭
		if err := c.rwc.Close(); err != nil {
			s.error(err)
		}
	}
	s.afterShutdown()
	s.debug("modbus: server closed")
	return nil
}

func (s *Server) DownloadCommand(remoteAddr string, in []byte) (out []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		// 如果活动链接中存在对应remoteAddr的链接，说明设备已经上线
		if c.remoteAddr == remoteAddr {
			// 每个链接，每次只进行一次下发命令，一次下发命令结束之后，再进行下一次
			// 判断是否在下发命令当中
			if c.inDownloading.isSet() {
				return nil, ErrDownloadCmdTooBusy
			}

			c.inDownloading.setTrue()

			if err = c.receiveCmd(in); err != nil {
				c.inDownloading.setFalse()
				return
			}

			ticker := time.NewTicker(c.server.DownloadCmdTimeout)

			select {
			case err = <-c.errorCh:
			case out = <-c.sendCmdRespCh:
			case <-ticker.C:
				ticker.Stop()
				err = ErrReceiveCmdResponseTimeout
			}

			c.inDownloading.setFalse()
			return
		}
	}
	return nil, ErrDeviceNotOnline
}

func (s *Server) trackConn(c *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// 关闭指定链接
func (s *Server) CloseConn(remoteAddr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		if c.remoteAddr == remoteAddr {
			if c.shuttingDown() {
				return ErrConnectionClosed
			}
			c.inShutdown.setTrue()
			c.server.logger.Debug(fmt.Sprintf("modbus: conn: %v closing", c.remoteAddr))
			// 关闭c.server()
			delete(s.activeConn, c)
			go c.server.afterConnClose(c.remoteAddr)
			if err := c.rwc.Close(); err != nil {
				return err
			}
			c.server.logger.Debug(fmt.Sprintf("modbus: conn: %v closed", c.remoteAddr))
			break
		}
	}
	return ErrDeviceNotOnline
}

// Create new connection from rwc.
func (s *Server) newConn(rwc net.Conn) *conn {
	return &conn{
		server:        s,
		rwc:           rwc,
		remoteAddr:    rwc.RemoteAddr().String(),
		receiveCmdCh:  make(chan []byte, 1),
		sendCmdRespCh: make(chan []byte, 1),
	}
}

func (c *conn) serve() {
	defer func() {
		if err := c.close(); err != nil {
			c.server.warn(err)
		}
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			c.server.error(fmt.Sprintf("modbus: panic serving %v: %v\n%s", c.remoteAddr, err, buf))
		}
	}()

	readCh := make(chan []byte)

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	go func(ch chan []byte) {
		for {
			// 设备主动上报
			buf, err := c.read()
			if err != nil {
				c.server.error(fmt.Sprintf(readFailedFormat, err, c.remoteAddr))
				c.cancel()
				return
			}
			ch <- buf
		}
	}(readCh)

	for {
		select {
		case <-ctx.Done():
			return
		// 服务器下发命令
		case cmd := <-c.receiveCmdCh:
			if err := c.write(cmd); err != nil {
				c.errorCh <- err
				c.server.error(fmt.Sprintf(writeFailedFormat, err, c.remoteAddr))
				// 如果写入失败，则断开链接
				return
			}

			ticker := time.NewTicker(c.server.DownloadCmdTimeout)
			select {
			case data := <-readCh:
				c.sendCmdRespCh <- data
			case <-ticker.C:
				ticker.Stop()
				c.errorCh <- ErrReceiveCmdResponseTimeout
			}
		case data := <-readCh:

			resp := c.server.uploadHandler(c.remoteAddr, data)

			if err := c.write(resp); err != nil {
				c.server.error(fmt.Sprintf(writeFailedFormat, err, c.remoteAddr))
				return
			}
		}
	}
}

func (c *conn) read() ([]byte, error) {
	buf := make([]byte, c.server.MaxBytes)
	if d := c.server.ReadTimeout; d != 0 {
		_ = c.rwc.SetReadDeadline(time.Now().Add(d))
	}
	readLen, err := c.rwc.Read(buf)
	if err != nil {
		return nil, err
	}
	_ = c.rwc.SetReadDeadline(time.Time{})
	if readLen > c.server.MaxBytes {
		return nil, ErrPacketTooLarge
	}
	buf = buf[:readLen]
	c.server.logger.Debug(fmt.Sprintf("modbus: read conn: %v,msg: 0x% x", c.remoteAddr, buf))
	return buf, nil
}

func (c *conn) write(buf []byte) (err error) {
	if d := c.server.WriteTimeout; d != 0 {
		_ = c.rwc.SetWriteDeadline(time.Now().Add(d))
	}
	_, err = c.rwc.Write(buf)
	if err != nil {
		return err
	}
	_ = c.rwc.SetWriteDeadline(time.Time{})
	c.server.logger.Debug(fmt.Sprintf("modbus: write conn: %v,msg: 0x% x", c.remoteAddr, buf))
	return
}

func (c *conn) close() error {
	if c.shuttingDown() {
		return ErrConnectionClosed
	}
	c.inShutdown.setTrue()
	c.server.logger.Debug(fmt.Sprintf("modbus: conn: %v closing", c.remoteAddr))
	c.server.trackConn(c, false)
	go c.server.afterConnClose(c.remoteAddr)
	if err := c.rwc.Close(); err != nil {
		return err
	}
	c.server.logger.Debug(fmt.Sprintf("modbus: conn: %v closed", c.remoteAddr))
	return nil
}

func (c *conn) shuttingDown() bool {
	return c.inShutdown.isSet()
}

func (c *conn) receiveCmd(in []byte) error {
	ticker := time.NewTicker(c.server.DownloadCmdTimeout)
	defer ticker.Stop()
	select {
	case c.receiveCmdCh <- in:
		return nil
	case <-ticker.C:
		c.server.error(fmt.Sprintf("modbus: command download timeout,conn: %v ", c.remoteAddr))
		return ErrDownloadCmdTimeout
	}
}
