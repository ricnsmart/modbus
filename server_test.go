package modbus

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

func TestServer_Serve(t *testing.T) {

	s := NewServer()

	s.SetUploadHandler(func(remoteAddr string, in []byte) []byte {
		log.Println(string(in))
		return nil
	})

	s.SetAfterConnClose(func(remoteAddr string) {
		// do something
	})

	s.SetAfterShutdown(func() {
		// do something
	})

	go func() {
		err := s.StartServer(":65007")
		if err != nil {
			log.Print(err.Error())
		}
	}()

	// gracefully shutdown
	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 10 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	<-quit
	_ = s.Shutdown()
}
