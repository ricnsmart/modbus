package modbus

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type Handler func(data []byte, addr net.Addr, answer func(payload []byte) error)

type Server struct {
	addr string

	handle Handler

	readTimeout time.Duration

	writeTimeout time.Duration

	// 存储连接
	// 用于主动关闭连接
	connMap sync.Map

	// 按照remoteAddr存储下发数据通道
	downChMap sync.Map

	// 按照remoteAddr存储下发响应数据通道
	// 其中的通道需要用时创建、用完删除
	respChMap sync.Map

	// 设备上报数据，server一次读取多少个字节
	readSize int

	onConnClose func(addr net.Addr)
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
		readSize:     DefaultMaxReadSize,
	}
}

func (s *Server) SetReadTimeout(t time.Duration) {
	s.readTimeout = t
}

func (s *Server) SetWriteTimeout(t time.Duration) {
	s.writeTimeout = t
}

func (s *Server) SetMaxReadSize(size int) {
	s.readSize = size
}

func (s *Server) SetOnConnClose(f func(addr net.Addr)) {
	s.onConnClose = f
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

		go s.newConn(rwc).serve()
	}
}

func (s *Server) newConn(rwc net.Conn) *conn {

	c := &conn{
		server: s,
		rwc:    rwc,
		downCh: make(chan []byte),
	}

	s.downChMap.Store(rwc.RemoteAddr(), c.downCh)

	s.connMap.Store(rwc.RemoteAddr(), rwc)

	return c
}

func (s *Server) CloseConn(addr any) error {
	v, ok := s.connMap.Load(addr)
	if !ok {
		return errors.New("connection not found")
	}
	rwc := v.(net.Conn)
	defer s.connMap.Delete(addr)
	return rwc.Close()
}

func (s *Server) DownloadCommand(ctx context.Context, addr any, cmd []byte) ([]byte, error) {
	c, ok := s.downChMap.Load(addr)
	if !ok {
		return nil, errors.New("connection not found")
	}

	respCh := make(chan []byte)

	s.respChMap.Store(addr, respCh)
	defer s.respChMap.Delete(addr)

	downCh := c.(chan []byte)

	select {
	case <-ctx.Done():
		return nil, errors.New("download timeout")
	case downCh <- cmd:

	}

	select {
	case <-ctx.Done():
		return nil, errors.New("response timeout")
	case resp := <-respCh:
		return resp, nil
	}
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

	// downCh 用户向设备下发的数据
	downCh chan []byte

	// respCh 用于存放下发后设备的响应
	respCh chan []byte
}

func (c *conn) serve() {

	// upCh 设备上发的数据
	upCh := make(chan []byte)

	ctx, cancel := context.WithCancel(context.Background())

	// 设备离线
	// 同时这也是判断设备是否在线的依据
	defer func() {
		c.server.downChMap.Delete(c.rwc.RemoteAddr())
		c.server.connMap.Delete(c.rwc)
		c.server.onConnClose(c.rwc.RemoteAddr())
	}()

	go func() {
		for {
			_ = c.rwc.SetReadDeadline(time.Now().Add(c.server.readTimeout))

			buf := make([]byte, c.server.readSize)

			l, err := c.rwc.Read(buf)
			if err != nil {
				cancel()
				return
			}

			buf = buf[:l]

			upCh <- buf
			_ = c.rwc.SetReadDeadline(time.Time{})
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.server.downChMap.Delete(c.rwc.RemoteAddr())
			return
		case packet := <-upCh:
			go c.server.handle(packet, c.rwc.RemoteAddr(), func(payload []byte) error {
				if err := c.write(payload); err != nil {
					cancel()
					return err
				}
				return nil
			})
		case payload := <-c.downCh:
			if err := c.write(payload); err != nil {
				cancel()
				return
			}

			respCh, ok := c.server.respChMap.Load(c.rwc.RemoteAddr())
			if !ok {
				continue
			}

			// 超时时间，最长不应超过读取超时时
			// 否则读取到的就可能不是下发命令后设备的响应
			ticker := time.NewTicker(c.server.readTimeout)

			select {
			case <-ticker.C:

			case resp := <-upCh:
				respCh.(chan []byte) <- resp
			}
		}
	}
}

func (c *conn) write(buf []byte) error {
	_ = c.rwc.SetWriteDeadline(time.Now().Add(c.server.writeTimeout))

	defer c.rwc.SetReadDeadline(time.Time{})

	if _, err := c.rwc.Write(buf); err != nil {
		return err
	}

	return nil
}
