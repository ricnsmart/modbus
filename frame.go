package modbus

import (
	"encoding/binary"
)

// Framer is the interface that wraps Modbus frames.
type Framer interface {
	Bytes() []byte
	Copy() Framer
	GetData() []byte
	GetFunction() uint8
	SetException(exception *Exception)
	SetData(data []byte)
}

// GetException return the Modbus exception or Success (indicating not exception).
func GetException(frame Framer) (exception Exception) {
	function := frame.GetFunction()
	if (function & 0x80) != 0 {
		exception = Exception(frame.GetData()[0])
	}
	return exception
}

// SetDataWithRegisterAndNumber 用于读寄存器
// SetDataWithRegisterAndNumber sets the RTUFrame Data byte field to hold a register and number of registers
func SetDataWithRegisterAndNumber(frame Framer, register uint16, number uint16) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], number)
	frame.SetData(data)
}

// SetDataWithRegisterAndNumberAndBytes 仅用于写寄存器
// SetDataWithRegisterAndNumberAndBytes sets the TCPFrame Data byte field to hold a register and number of registers and coil bytes
func SetDataWithRegisterAndNumberAndBytes(frame Framer, register uint16, number uint16, bytes []byte) {
	data := make([]byte, 5+len(bytes))
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], number)
	data[4] = byte(len(bytes))
	copy(data[5:], bytes)
	frame.SetData(data)
}

// SetDateForControl 仅用于遥控操作寄存器
func SetDateForControl(frame Framer, register uint16, value uint16) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], value)
	frame.SetData(data)
}
