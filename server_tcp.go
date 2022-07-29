package modbus

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"time"
)

type Handler func(data []byte, writer func([]byte) error, addr net.Addr)

type Server struct {
	addr string

	handle Handler

	readTimeout time.Duration

	writeTimeout time.Duration

	sm sync.Map

	handleRemoteAddr func(addr net.Addr)

	maxReadSize int
}

var DefaultReadTimeout = 60 * time.Second

var DefaultWriteTimeout = 60 * time.Second

var DefaultMaxReadSize = 512

func NewServer(addr string, handle Handler) *Server {
	return &Server{
		addr:         addr,
		handle:       handle,
		readTimeout:  DefaultReadTimeout,
		writeTimeout: DefaultWriteTimeout,
		maxReadSize:  DefaultMaxReadSize,
	}
}

func (s *Server) SetReadTimeout(t time.Duration) {
	s.readTimeout = t
}

func (s *Server) SetWriteTimeout(t time.Duration) {
	s.writeTimeout = t
}

func (s *Server) SetMaxReadSize(size int) {
	s.maxReadSize = size
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	defer ln.Close()

	for {
		rwc, err := ln.Accept()
		if err != nil {
			return err
		}

		c := s.newConn(rwc)

		s.sm.Store(rwc.RemoteAddr(), c.north)

		go c.serve()
	}
}

func (s *Server) newConn(rwc net.Conn) *conn {
	return &conn{
		server: s,
		rwc:    rwc,
		north:  make(chan []byte),
	}
}

func (s *Server) DownloadCMD(ctx context.Context, addr net.Addr, cmd []byte) error {
	c, ok := s.sm.Load(addr)
	if !ok {
		return errors.New("connection not found")
	}

	north := c.(chan []byte)

	select {
	case <-ctx.Done():
		return errors.New("download timeout")
	case north <- cmd:

	}

	return nil
}

type conn struct {
	// server is the server on which the connection arrived.
	// Immutable; never nil.
	server *Server

	// rwc is the underlying network connection.
	// This is never wrapped by other types and is the value given out
	// to CloseNotifier callers. It is usually of type *net.TCPConn or
	// *tls.conn.
	rwc net.Conn

	north chan []byte
}

func (c *conn) serve() {

	south := make(chan []byte)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			_ = c.rwc.SetReadDeadline(time.Now().Add(c.server.readTimeout))

			buf := make([]byte, c.server.maxReadSize)

			l, err := c.rwc.Read(buf)
			if err != nil {
				cancel()
				log.Println(err)
				return
			}

			buf = buf[:l]

			south <- buf
			_ = c.rwc.SetReadDeadline(time.Time{})
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.server.sm.Delete(c.rwc.RemoteAddr())
			return
		case s := <-south:
			go c.server.handle(s, c.write, c.rwc.RemoteAddr())
		case n := <-c.north:
			if err := c.write(n); err != nil {
				return
			}
		}
	}
}

func (c *conn) write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeTimeout))

	defer c.rwc.SetReadDeadline(time.Time{})

	if _, err := c.rwc.Write(buf); err != nil {
		log.Println(err)
		return err
	}

	return nil
}
