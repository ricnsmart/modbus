package modbus

import (
	"log"
	"testing"
)

func TestRTUFrame_Bytes(t *testing.T) {
	// 读取寄存器
	frame := &RTUFrame{
		Address:  0x01,
		Function: Read,
	}
	SetDataWithRegisterAndNumber(frame, 0x03, 7)
	log.Printf("%v", frame.Bytes())
}
