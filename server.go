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
	Handler func(remoteAddr string, in []byte) []byte

	// @param remoteAddr string
	// @return [][]byte 需要下发的命令组
	BuildCommands func(remoteAddr string) [][]byte

	// @param remoteAddr 设备远程地址 ip:port
	// @param response *sync.Map 此次命令组执行的响应结果
	// 其中sync.Map的key为命令组的索引值，value为响应结果，其类型为interface{}，有两种情况：error或者 []byte
	HandleCommandsResponse func(remoteAddr string, response map[int]interface{})

	Server struct {
		// Addr optionally specifies the TCP address for the server to listen on,
		// in the form "host:port".
		// The service names are defined in RFC 6335 and assigned by IANA.
		// See net.Dial for details of the address format.
		Addr string

		// 处理设备主动上行报文
		Handler Handler

		// ReadTimeout is the maximum duration for reading the entire
		// request, including the body.
		//
		// Because ReadTimeout does not let Handlers make per-request
		// decisions on each request body's acceptable deadline or
		// upload rate, most users will prefer to use
		// ReadHeaderTimeout. It is valid to use them both.
		ReadTimeout time.Duration

		// WriteTimeout is the maximum duration before timing out
		// writes of the response. It is reset whenever a new
		// request's header is read. Like ReadTimeout, it does not
		// let Handlers make decisions on a per-request basis.
		WriteTimeout time.Duration

		// 下发命令超时
		DownloadCmdTimeout time.Duration

		// 注册命令超时设置
		RegisteredCmdTimeout time.Duration

		// 一次性读取字节流的最大长度
		MaxBytes int

		// 保存所有活动连接
		mu         sync.Mutex
		listeners  map[*net.Listener]struct{}
		activeConn map[*conn]struct{}
		doneChan   chan struct{}
		inShutdown atomicBool // true when server is in shutdown
		onShutdown []func()

		onConnClose []func(remoteAddr string)

		// 每个链接都会按指定间隔循环执行的命令组
		loopCommands *loopCommands

		// 每个链接都会执行一次的命令组列表
		// 包含多组命令组
		onceCommandsList []*onceCommands

		logger Logger
	}

	// 指定间隔循环执行的命令组
	// 在每个链接状态
	// It should be called in a loop.
	loopCommands struct {
		buildCommands BuildCommands

		// 命令执行后对响应结果的处理方法
		handleCommandsResponse HandleCommandsResponse

		// 命令下发间隔
		commandsInterval time.Duration
	}

	// 只执行一次的命令组
	onceCommands struct {
		buildCommands BuildCommands

		handleCommandsResponse HandleCommandsResponse
	}
)

func NewCustomServer(readTimeout time.Duration, writeTimeout time.Duration, downloadCmdTimeout time.Duration, maxBytes int, logger Logger) *Server {
	return &Server{ReadTimeout: readTimeout, WriteTimeout: writeTimeout, DownloadCmdTimeout: downloadCmdTimeout, MaxBytes: maxBytes, logger: logger}
}

func (s *Server) getDoneChan() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getDoneChanLocked()
}

func (s *Server) getDoneChanLocked() chan struct{} {
	if s.doneChan == nil {
		s.doneChan = make(chan struct{})
	}
	return s.doneChan
}

func (s *Server) closeDoneChanLocked() {
	ch := s.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(ch)
	}
}

// Close immediately closes all active net.Listeners and any
// connections in state StateNew, StateActive, or StateIdle. For a
// graceful shutdown, use Shutdown.
//
// Close does not attempt to close (and does not even know about)
// any hijacked connections, such as WebSockets.
//
// Close returns any error returned from closing the Server's
// underlying Listener(s).
func (s *Server) Close() error {
	s.inShutdown.setTrue()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeDoneChanLocked()
	err := s.closeListenersLocked()
	for c := range s.activeConn {
		_ = c.rwc.Close()
		delete(s.activeConn, c)
	}
	return err
}

// shutdownPollInterval is how often we poll for quiescence
// during Server.Shutdown. This is lower during tests, to
// speed up tests.
// Ideally we could find a solution that doesn't involve polling,
// but which also doesn't have a high runtime cost (and doesn't
// involve any contentious mutexes), but that is left as an
// exercise for the reader.
var shutdownPollInterval = 500 * time.Millisecond

// Shutdown gracefully shuts down the server without interrupting any
// active connections. Shutdown works by first closing all open
// listeners, then closing all idle connections, and then waiting
// indefinitely for connections to return to idle and then shut down.
// If the provided context expires before the shutdown is complete,
// Shutdown returns the context's error, otherwise it returns any
// error returned from closing the Server's underlying Listener(s).
//
// When Shutdown is called, Serve, ListenAndServe, and
// ListenAndServeTLS immediately return ErrServerClosed. Make sure the
// program doesn't exit and waits instead for Shutdown to return.
//
// Shutdown does not attempt to close nor wait for hijacked
// connections such as WebSockets. The caller of Shutdown should
// separately notify such long-lived connections of shutdown and wait
// for them to close, if desired. See RegisterOnServerShutdown for a way to
// register shutdown notification functions.
//
// Once Shutdown has been called on a server, it may not be reused;
// future calls to methods such as Serve will return ErrServerClosed.
func (s *Server) Shutdown(ctx context.Context) error {
	s.inShutdown.setTrue()

	s.mu.Lock()
	lnerr := s.closeListenersLocked()
	s.closeDoneChanLocked()
	for _, f := range s.onShutdown {
		go f()
	}
	s.mu.Unlock()

	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		if s.closeIdleConns() && s.numListeners() == 0 {
			return lnerr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RegisterOnServerShutdown registers a function to call on Shutdown.
// This can be used to gracefully shutdown connections
// This function should start protocol-specific graceful shutdown,
// but should not wait for shutdown to complete.
func (s *Server) RegisterOnServerShutdown(f func()) {
	s.mu.Lock()
	s.onShutdown = append(s.onShutdown, f)
	s.mu.Unlock()
}

func (s *Server) RegisterOnConnClose(f func(remoteAddr string)) {
	s.onConnClose = append(s.onConnClose, f)
}

func (s *Server) numListeners() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.listeners)
}

// closeIdleConns closes all idle connections and reports whether the
// server is quiescent.
func (s *Server) closeIdleConns() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	quiescent := true
	for c := range s.activeConn {
		st, unixSec := c.getState()
		// Issue 22682: treat StateNew connections as if
		// they're idle if we haven't read the first request's
		// header in over 5 seconds.
		if st == StateNew && unixSec < time.Now().Unix()-5 {
			st = StateIdle
		}
		if st != StateIdle || unixSec == 0 {
			// Assume unixSec == 0 means it's a very new
			// connection, without state set yet.
			quiescent = false
			continue
		}
		_ = c.rwc.Close()
		delete(s.activeConn, c)
	}
	return quiescent
}

func (s *Server) closeListenersLocked() error {
	var err error
	for ln := range s.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}

// A ConnState represents the state of a client connection to a server.
// It's used by the optional Server.ConnState hook.
type ConnState int

const (
	// StateNew represents a new connection that is expected to
	// send a request immediately. Connections begin at this
	// state and then transition to either StateActive or
	// StateClosed.
	StateNew ConnState = iota

	StateUploading

	StateDownloading

	// StateIdle represents a connection that has finished
	// handling a request and is in the keep-alive state, waiting
	// for a new request. Connections transition from StateIdle
	// to either StateActive or StateClosed.
	StateIdle

	// StateClosed represents a closed connection.
	// This is a terminal state.
	StateClosed
)

var stateName = map[ConnState]string{
	StateNew:         "new",
	StateUploading:   "uploading",
	StateDownloading: "downloading",
	StateIdle:        "idle",
	StateClosed:      "closed",
}

func (c ConnState) String() string {
	return stateName[c]
}

func (c *conn) setState(state ConnState) {
	srv := c.server
	switch state {
	case StateNew:
		srv.trackConn(c, true)
	case StateClosed:
		srv.trackConn(c, false)
	}
	if state > 0xff || state < 0 {
		panic("internal error")
	}
	packedState := uint64(time.Now().Unix()<<8) | uint64(state)
	atomic.StoreUint64(&c.curState.atomic, packedState)
}

func (c *conn) getState() (state ConnState, unixSec int64) {
	packedState := atomic.LoadUint64(&c.curState.atomic)
	return ConnState(packedState & 0xff), int64(packedState >> 8)
}

type atomicBool int32

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

const (
	defaultMaxBytes             = 500
	defaultDownloadTimeout      = 20 * time.Second
	defaultWriteTimeout         = 1 * time.Minute
	defaultReadTimeout          = 4 * time.Minute
	defaultRegisteredCmdTimeout = 2 * time.Minute
	readFailedFormat            = "read failed: %v,conn: %v"
	writeFailedFormat           = "write failed: %v,conn: %v"
)

// ErrServerClosed is returned by the Server's Serve, ServeTLS, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("modbus: Server already closed")

var ErrPacketTooLarge = errors.New("modbus: packet too large")

var ErrDeviceNotOnline = errors.New("modbus: device not online")

var ErrDownloadCmdTooBusy = errors.New("modbus: downloading command,try later")

var ErrDownloadCmdTimeout = errors.New("modbus: download command timeout")

var ErrReceiveCmdResponseTimeout = errors.New("modbus: receive command response timeout")

func NewServer() *Server {
	return &Server{
		logger:               NewDefaultLogger(),
		MaxBytes:             defaultMaxBytes,
		ReadTimeout:          defaultReadTimeout,
		WriteTimeout:         defaultWriteTimeout,
		DownloadCmdTimeout:   defaultDownloadTimeout,
		RegisteredCmdTimeout: defaultRegisteredCmdTimeout,
	}
}

func (s *Server) SetMaxBytes(max int) {
	s.MaxBytes = max
}

func (s *Server) SetReadTimeout(t time.Duration) {
	s.ReadTimeout = t
}

func (s *Server) SetWriteTimeout(t time.Duration) {
	s.WriteTimeout = t
}

func (s *Server) SetDownloadCmdTimeout(t time.Duration) {
	s.DownloadCmdTimeout = t
}

func (s *Server) SetRegisteredCmdTimeout(t time.Duration) {
	s.RegisteredCmdTimeout = t
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

func (s *Server) Start(addr string, Handler Handler) error {
	s.Addr = addr
	s.Handler = Handler
	return s.ListenAndServe()
}

func (s *Server) ListenAndServe() error {
	if s.shuttingDown() {
		return ErrServerClosed
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

func (s *Server) Serve(l net.Listener) error {
	defer l.Close()

	if !s.trackListener(&l, true) {
		return ErrServerClosed
	}
	defer s.trackListener(&l, false)

	var tempDelay time.Duration // how long to sleep on accept failure

	for {
		rwc, err := l.Accept()
		if err != nil {
			select {
			case <-s.getDoneChan():
				return ErrServerClosed
			default:
			}
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
		c.setState(StateNew) // before Serve can return
		go c.serve()
	}
}

// trackListener adds or removes a net.Listener to the set of tracked
// listeners.
//
// We store a pointer to interface in the map set, in case the
// net.Listener is not comparable. This is safe because we only call
// trackListener via Serve and can track+defer untrack the same
// pointer to local variable there. We never need to compare a
// Listener from another caller.
//
// It reports whether the server is still up (not Shutdown or Closed).
func (s *Server) trackListener(ln *net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.listeners[ln] = struct{}{}
	} else {
		delete(s.listeners, ln)
	}
	return true
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

// 注册一组命令
// 这组命令在每个链接上都会执行一次
// 命令在链接空闲时才会执行
// 注意：可以注册多组命令
func (s *Server) RegisterOnceCommands(b BuildCommands, f HandleCommandsResponse) {
	s.onceCommandsList = append(s.onceCommandsList, &onceCommands{
		buildCommands:          b,
		handleCommandsResponse: f,
	})
}

// 注册一组循环执行的命令
// 这组命令在每个链接上每隔一个interval的间隔都会执行一次
// 命令在链接空闲时才会执行
// 注意：只需注册一次
func (s *Server) RegisterLoopCommands(b BuildCommands, interval time.Duration, f HandleCommandsResponse) {
	s.loopCommands = &loopCommands{
		buildCommands:          b,
		handleCommandsResponse: f,
		commandsInterval:       interval,
	}
}

// 一次性对所有活动链接下发一条命令
// @return sm *sync.Map key为c.remoteAddr  value为下发结果：error或者[]byte
func (s *Server) DownloadOneCommandToAllConn(in []byte) (response *sync.Map) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wg := sync.WaitGroup{}
	wg.Add(len(s.activeConn))

	for c := range s.activeConn {
		go func(c *conn) {
			defer wg.Done()
			// 这里的时间不能太短
			t := time.NewTicker(c.server.RegisteredCmdTimeout)
			for {
				select {
				case <-t.C:
					t.Stop()
					response.Store(c.remoteAddr, ErrDownloadCmdTimeout)
					return
				default:
					// 因为遍历的是activeConn，而conn的连接状态设置又是被s.mu锁住的
					// 所以这里state不太可能是StateClosed
					state, _ := c.getState()
					// 如果设备已经在下发命令，则一直等待直到状态改变或者超时
					if state != StateIdle {
						continue
					}

					if err := c.receiveCmd(in); err != nil {
						response.Store(c.remoteAddr, err)
						return
					}

					ticker := time.NewTicker(c.server.DownloadCmdTimeout)

					select {
					case err := <-c.errorCh:
						response.Store(c.remoteAddr, err)
					case out := <-c.sendCmdRespCh:
						response.Store(c.remoteAddr, out)
					case <-ticker.C:
						ticker.Stop()
						response.Store(c.remoteAddr, ErrReceiveCmdResponseTimeout)
					}

					return
				}
			}
		}(c)
	}
	wg.Wait()
	return
}

// 一次性针对单个链接下发单个命令
func (s *Server) DownloadOneCommand(remoteAddr string, in []byte) (response []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		// 如果活动链接中存在对应remoteAddr的链接，说明设备已经上线
		if c.remoteAddr == remoteAddr {
			// 因为遍历的是activeConn，而conn的连接状态设置又是被s.mu锁住的
			// 所以这里state不太可能是StateClosed
			state, _ := c.getState()
			// 每个链接，每次只进行一次下发命令，一次下发命令结束之后，再进行下一次
			// 如果下发时设备已经在下发命令状态，则中止此次下发
			// 判断是否在下发命令当中
			if state == StateDownloading {
				return nil, ErrDownloadCmdTooBusy
			}

			if err = c.receiveCmd(in); err != nil {
				return
			}

			ticker := time.NewTicker(c.server.DownloadCmdTimeout)

			select {
			case err = <-c.errorCh:
			case response = <-c.sendCmdRespCh:
			case <-ticker.C:
				ticker.Stop()
				err = ErrReceiveCmdResponseTimeout
			}

			return
		}
	}
	return nil, ErrDeviceNotOnline
}

// 关闭指定链接
func (s *Server) CloseConn(remoteAddr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		if c.remoteAddr == remoteAddr {
			c.server.logger.Debug(fmt.Sprintf("modbus: conn: %v closing", c.remoteAddr))
			packedState := uint64(time.Now().Unix()<<8) | uint64(StateClosed)
			atomic.StoreUint64(&c.curState.atomic, packedState)
			// 关闭c.server()
			delete(s.activeConn, c)
			for _, f := range c.server.onConnClose {
				go f(c.remoteAddr)
			}
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

// A conn represents the server side of an tcp connection.
type conn struct {
	// server is the server on which the connection arrived.
	// Immutable; never nil.
	server *Server

	// rwc is the underlying network connection.
	// This is never wrapped by other types and is the value given out
	// to CloseNotifier callers. It is usually of type *net.TCPConn or
	// *tls.conn.
	rwc net.Conn

	curState struct{ atomic uint64 } // packed (unixtime<<8|uint8(ConnState))

	// cancelCtx cancels the connection-level context.
	cancelCtx context.CancelFunc

	// remoteAddr is rwc.RemoteAddr().String()
	// 可以用来作为标识链接的唯一值
	remoteAddr string

	receiveCmdCh chan []byte // 接收命令

	sendCmdRespCh chan []byte // 发送命令响应

	errorCh chan error
}

func (c *conn) serve() {
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			c.server.error(fmt.Sprintf("modbus: panic serving %v: %v\n%s", c.remoteAddr, err, buf))
		}
		if state, _ := c.getState(); state != StateClosed {
			if err := c.close(); err != nil {
				c.server.error(fmt.Sprintf("modbus: close conn failed: %v", err))
			}
			c.setState(StateClosed)
		}
		c.server.debug("modbus: connection main runtime closed")
	}()

	readCh := make(chan []byte)

	ctx, cancelCtx := context.WithCancel(context.Background())
	c.cancelCtx = cancelCtx
	defer cancelCtx()

	go func(ch chan []byte) {
		defer func() {
			c.server.debug("modbus: read runtime closed")
		}()
		for {
			// 设备主动上报
			buf, err := c.read()
			if err != nil {
				c.server.error(fmt.Sprintf(readFailedFormat, err, c.remoteAddr))
				c.cancelCtx()
				return
			}
			ch <- buf
		}
	}(readCh)

	if len(c.server.onceCommandsList) != 0 {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					c.server.error(fmt.Sprintf("modbus: once commands runtime panic serving %v: %v\n%s", c.remoteAddr, err, buf))
				}
				c.server.debug("modbus: once commands runtime closed")
			}()

			for _, oc := range c.server.onceCommandsList {
				resp := make(map[int]interface{})
			cmdLoop:
				for index, cmd := range oc.buildCommands(c.remoteAddr) {
					state, _ := c.getState()
					ticker := time.NewTicker(c.server.RegisteredCmdTimeout)
				loop:
					for {
						select {
						case <-ticker.C:
							ticker.Stop()
							resp[index] = ErrDownloadCmdTimeout
							c.server.debug(ErrDownloadCmdTimeout.Error())
							continue cmdLoop
						case <-ctx.Done():
							return
						default:
							// 当设备处于空闲时才开始执行loopCommands
							if state == StateIdle {
								break loop
							}
						}
					}

					if err := c.receiveCmd(cmd); err != nil {
						c.server.debug(err)
						return
					}

					select {
					case err := <-c.errorCh:
						resp[index] = err
						c.server.debug(err)
					case out := <-c.sendCmdRespCh:
						resp[index] = out
					case <-ticker.C:
						ticker.Stop()
						resp[index] = ErrReceiveCmdResponseTimeout
					}
				}
				oc.handleCommandsResponse(c.remoteAddr, resp)
			}
		}()
	}

	if c.server.loopCommands != nil {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					c.server.error(fmt.Sprintf("modbus: loop commands runtime panic serving %v: %v\n%s", c.remoteAddr, err, buf))
				}
				c.server.debug("modbus: loop commands runtime closed")
			}()

			for {
				resp := make(map[int]interface{})
			cmdLoop:
				for index, cmd := range c.server.loopCommands.buildCommands(c.remoteAddr) {
					state, _ := c.getState()
					ticker := time.NewTicker(c.server.RegisteredCmdTimeout)
				loop:
					for {
						select {
						case <-ticker.C:
							ticker.Stop()
							resp[index] = ErrDownloadCmdTimeout
							c.server.debug(ErrDownloadCmdTimeout.Error())
							continue cmdLoop
						case <-ctx.Done():
							return
						default:
							// 当设备处于空闲时才开始执行loopCommands
							if state == StateIdle {
								break loop
							}
						}
					}

					if err := c.receiveCmd(cmd); err != nil {
						c.server.debug(err)
						return
					}

					select {
					case err := <-c.errorCh:
						resp[index] = err
						c.server.debug(err)
					case out := <-c.sendCmdRespCh:
						resp[index] = out
					case <-ticker.C:
						ticker.Stop()
						resp[index] = ErrReceiveCmdResponseTimeout
					}
				}
				c.server.loopCommands.handleCommandsResponse(c.remoteAddr, resp)
				time.Sleep(c.server.loopCommands.commandsInterval)
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return
		// 服务器下发命令
		case cmd := <-c.receiveCmdCh:
			c.setState(StateDownloading)
			if err := c.write(cmd); err != nil {
				c.errorCh <- err
				c.server.error(fmt.Sprintf(writeFailedFormat, err, c.remoteAddr))
				// 如果写入失败，则断开链接
				return
			}
			// 这里的超时必须比conn read time out短，否则可能读上来一个主动上报的包，而不是写入的响应
			ticker := time.NewTicker(c.server.DownloadCmdTimeout)
			select {
			case data := <-readCh:
				c.sendCmdRespCh <- data
			case <-ticker.C:
				ticker.Stop()
				c.errorCh <- ErrReceiveCmdResponseTimeout
			}
		case out := <-readCh:

			// If we read any bytes off the wire, we're active.
			c.setState(StateUploading)

			resp := c.server.Handler(c.remoteAddr, out)

			// 部分情况下，上报不需要回应
			if resp != nil {
				if err := c.write(resp); err != nil {
					c.server.error(fmt.Sprintf(writeFailedFormat, err, c.remoteAddr))
					return
				}
			}
		}
		c.setState(StateIdle)
	}
}

// Close the connection.
func (c *conn) close() error {
	for _, f := range c.server.onConnClose {
		go f(c.remoteAddr)
	}
	return c.rwc.Close()
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
