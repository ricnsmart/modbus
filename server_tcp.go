package modbus

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

type ErrorLevel uint8

const (
	Silent ErrorLevel = iota
	INFO
	ERROR
	DEBUG
)

var DefaultReadDeadLine = 120 * time.Second

var DefaultWriteDeadLine = 120 * time.Second

var DefaultReadSize uint16 = 512

type Server struct {
	address       string
	serve         func(conn *Conn)
	readSize      uint16
	readDeadLine  time.Duration
	writeDeadLine time.Duration

	logLevel ErrorLevel

	// 打印连接数量的时间间隔，默认5分钟
	interval time.Duration
}

func NewServer(address string) *Server {
	return &Server{
		address:       address,
		readSize:      DefaultReadSize,
		readDeadLine:  DefaultReadDeadLine,
		writeDeadLine: DefaultWriteDeadLine,
		logLevel:      ERROR,
		interval:      5 * time.Minute,
	}
}

func (s *Server) SetServe(serve func(conn *Conn)) {
	s.serve = serve
}

func (s *Server) SetReadDeadline(readDeadLine time.Duration) {
	s.readDeadLine = readDeadLine
}

func (s *Server) SetWriteDeadline(writeDeadLine time.Duration) {
	s.writeDeadLine = writeDeadLine
}

func (s *Server) SetMaxReadSize(size uint16) {
	s.readSize = size
}

func (s *Server) SetInterval(interval time.Duration) {
	s.interval = interval
}

func (s *Server) SetLogLevel(logLevel ErrorLevel) {
	s.logLevel = logLevel
}

func (s *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	defer listener.Close()

	var counter AtomicCounter

	if s.logLevel >= INFO {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ticker.Stop()
					ticker = time.NewTicker(s.interval)
					log.Printf("INF connections: %v\n", counter.Load())
				}
			}
		}()
	}
	for {
		rwc, err := listener.Accept()
		if err != nil {
			return err
		}

		counter.Add(1)

		go func() {
			s.serve(&Conn{rwc: rwc, server: s})
			counter.Add(-1)
		}()
	}
}

type Conn struct {
	rwc    net.Conn
	server *Server
	mu     sync.Mutex
}

func (c *Conn) read() ([]byte, error) {
	_ = c.rwc.SetReadDeadline(time.Now().Add(c.server.readDeadLine))

	defer c.rwc.SetReadDeadline(time.Time{})

	buf := make([]byte, c.server.readSize)

	l, err := c.rwc.Read(buf)
	if err != nil {
		return nil, err
	}

	if c.server.logLevel == DEBUG {
		log.Printf("DBG %v read: % x\n", c.rwc.RemoteAddr(), buf[:l])
	}

	return buf[:l], nil
}

func (c *Conn) write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeDeadLine))

	defer c.rwc.SetWriteDeadline(time.Time{})

	if c.server.logLevel == DEBUG {
		log.Printf("DBG %v write: % x\n", c.rwc.RemoteAddr(), buf)
	}

	_, err := c.rwc.Write(buf)

	return err
}

func (c *Conn) Close() error {
	return c.rwc.Close()
}

func (c *Conn) Addr() net.Addr {
	return c.rwc.RemoteAddr()
}

func (c *Conn) NewRequest(frame Framer) (Framer, error) {
	c.mu.Lock()
	defer func() {
		// 控制请求频率，减少粘包
		time.Sleep(100)
		c.mu.Unlock()
	}()

	if err := c.write(frame.Bytes()); err != nil {
		return nil, err
	}

	buf, err := c.read()
	if err != nil {
		return nil, err
	}
	respFrame, err := NewRTUFrame(buf)
	if err != nil {
		return nil, err
	}
	if exception := GetException(respFrame); exception != Success {
		return nil, exception
	}
	return respFrame, nil
}

func (c *Conn) Ask() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read()
}

// Download 只下发命令，不等待设备响应
// 比如修改设备指向，设备将不会回应，如果等待，需要等待到连接超时
func (c *Conn) Download(frame Framer) error {
	c.mu.Lock()
	defer func() {
		// 控制请求频率，减少粘包
		time.Sleep(100)
		c.mu.Unlock()
	}()
	return c.write(frame.Bytes())
}
