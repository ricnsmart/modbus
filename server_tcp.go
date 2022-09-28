package modbus

import (
	"log"
	"net"
	"time"
)

var DefaultReadDeadLine = 60 * time.Second

var DefaultWriteDeadLine = 60 * time.Second

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
		log.Printf("% x\n", buf[:l])
	}

	return buf[:l], nil
}

func (c *Conn) Write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeDeadLine))

	defer c.rwc.SetWriteDeadline(time.Time{})

	if c.server.debug {
		log.Printf("% x\n", buf)
	}

	_, err := c.rwc.Write(buf)

	return err
}

func (c *Conn) Close() {
	_ = c.rwc.Close()
}

func (c *Conn) DownloadCommand(frame Framer) (Framer, error) {
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
