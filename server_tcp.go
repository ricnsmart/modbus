package modbus

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ErrorLevel int

const (
	INFO ErrorLevel = iota + 1
	ERROR
	DEBUG
)

var DefaultReadDeadLine = 120 * time.Second

var DefaultWriteDeadLine = 120 * time.Second

var DefaultReadSize = 512

type Server struct {
	address       string
	serve         func(conn *Conn)
	readSize      int
	readDeadLine  time.Duration
	writeDeadLine time.Duration

	logLevel ErrorLevel

	// 打印连接数量的时间间隔，默认5分钟
	interval time.Duration

	// 连接总数
	total int32
}

func NewServer(address string) *Server {
	return &Server{
		address:       address,
		readSize:      DefaultReadSize,
		readDeadLine:  DefaultReadDeadLine,
		writeDeadLine: DefaultWriteDeadLine,
		logLevel:      ERROR,
		interval:      5 * time.Minute,
		total:         0,
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

func (s *Server) SetInterval(interval time.Duration) {
	s.interval = interval
}

func (s *Server) SetLogLevel(logLevel ErrorLevel) {
	s.logLevel = logLevel
}

func (s *Server) ListenTCP() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	defer listener.Close()

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
					log.Printf("INFO connections: %v\n", s.total)
				}
			}
		}()
	}
	for {
		rwc, err := listener.Accept()
		if err != nil {
			return err
		}

		atomic.AddInt32(&s.total, 1)

		go func() {
			defer func() {
				if err := rwc.Close(); err != nil {
					if s.logLevel >= ERROR {
						log.Printf("ERROR %v %v\n", rwc.RemoteAddr(), err)
					}
					return
				}
				atomic.AddInt32(&s.total, -1)
			}()
			s.serve(&Conn{rwc: rwc, server: s})
		}()
	}
}

type Conn struct {
	rwc    net.Conn
	server *Server
	mu     sync.Mutex
}

func (c *Conn) Read() ([]byte, error) {
	_ = c.rwc.SetReadDeadline(time.Now().Add(c.server.readDeadLine))

	defer c.rwc.SetReadDeadline(time.Time{})

	buf := make([]byte, c.server.readSize)

	l, err := c.rwc.Read(buf)
	if err != nil {
		return nil, err
	}

	if c.server.logLevel == DEBUG {
		log.Printf("DEBUG %v Read: % x\n", c.rwc.RemoteAddr(), buf[:l])
	}

	return buf[:l], nil
}

func (c *Conn) Write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeDeadLine))

	defer c.rwc.SetWriteDeadline(time.Time{})

	if c.server.logLevel == DEBUG {
		log.Printf("DEBUG %v write: % x\n", c.rwc.RemoteAddr(), buf)
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

func (c *Conn) Lock() {
	c.mu.Lock()
}

func (c *Conn) Unlock() {
	c.mu.Unlock()
}

func (c *Conn) NewRequest(frame Framer) (Framer, error) {
	c.mu.Lock()
	defer func() {
		// 控制请求频率，减少粘包
		time.Sleep(100)
		c.mu.Unlock()
	}()

	if err := c.Write(frame.Bytes()); err != nil {
		return nil, err
	}

	buf, err := c.Read()
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
