package modbus

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestServer_Serve(t *testing.T) {

	s := NewServer()

	s.RegisterOnConnClose(func(remoteAddr string) {
		// do something
	})

	s.RegisterOnServerShutdown(func() {

	})

	cmd1 := make([]byte, 100)
	cmd2 := make([]byte, 100)
	commands := make([][]byte, 0)
	commands = append(commands, cmd1, cmd2)
	s.ExecuteStandingCommands(commands, 2*time.Minute, func(remoteAddr string, response *sync.Map) {
		response.Range(func(key, value interface{}) bool {
			// 索引，和commands同步
			index := key.(int)
			fmt.Println(index)
			switch value.(type) {
			case error:
				// 业务代码
			case []byte:
				// 业务代码
			default:
				panic("未知的响应类型")
			}
			return true
		})

	})

	go func() {
		err := s.Start(":65007", func(remoteAddr string, in []byte) []byte {
			return nil
		})
		if err != nil {
			log.Print(err.Error())
		}
	}()

	cmd := make([]byte, 100)

	// 批量下发命令
	resp1 := s.DownloadOneCommandToAllConn(cmd)

	resp1.Range(func(key, value interface{}) bool {
		// do something
		return true
	})

	// 针对单个链接下发单个命令
	resp, err := s.DownloadOneCommand("1.1.1.1", cmd)
	if err != nil {
		log.Fatal()
	}
	fmt.Print(resp)

	// gracefully shutdown
	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 10 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	<-quit
	_ = s.Shutdown(context.Background())
}
