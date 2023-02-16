package modbus

import (
	"log"
	"net"
	"sync"
	"time"
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
	debug         bool
}

func NewServer(address string) *Server {
	return &Server{
		address:       address,
		readSize:      DefaultReadSize,
		readDeadLine:  DefaultReadDeadLine,
		writeDeadLine: DefaultWriteDeadLine,
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

func (s *Server) SetDebug(debug bool) {
	s.debug = debug
}

func (s *Server) ListenTCP() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	defer listener.Close()

	for {
		rwc, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.serve(&Conn{rwc: rwc, server: s})
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

	if c.server.debug {
		log.Printf("DEBUG %v Read: % x\n", c.rwc.RemoteAddr(), buf[:l])
	}

	return buf[:l], nil
}

func (c *Conn) Write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeDeadLine))

	defer c.rwc.SetWriteDeadline(time.Time{})

	if c.server.debug {
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
