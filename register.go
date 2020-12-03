package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
)

type Register interface {
	GetName() string
	GetStart() uint16
	GetNum() uint16
}

type Readable interface {
	Decode(data []byte) interface{}
}

type Writable interface {
	Verify(value interface{}) error
	Encode(value interface{}) []byte
}

type ReadableRegister interface {
	Register
	Readable
}

type ReadableRegisters []ReadableRegister

func (rs ReadableRegisters) Decode(data []byte) map[string]interface{} {
	m := make(map[string]interface{})
	for _, r := range rs {
		name := r.GetName()
		start := r.GetStart()
		end := start + r.GetNum()*2
		m[name] = r.Decode(data[start:end])
	}
	return m
}

type Float32RoRegister struct {
	Name  string
	Start uint16
	Num   uint16
	Parse func(data float32) interface{}
}

func (f *Float32RoRegister) GetName() string {
	return f.Name
}

func (f *Float32RoRegister) GetStart() uint16 {
	return f.Start
}

func (f *Float32RoRegister) GetNum() uint16 {
	return f.Num
}

func (f *Float32RoRegister) Decode(data []byte) interface{} {
	bits := binary.LittleEndian.Uint32(data)
	f32 := math.Float32frombits(bits)
	if f.Parse != nil {
		return f.Parse(f32)
	}
	return f32
}

type WritableRegister interface {
	Register
	Writable
}

type WritableRegisters []WritableRegister

type ReadableAndWritableRegister interface {
	Register
	Readable
	Writable
}

type ReadableAndWritableRegisters []ReadableAndWritableRegister

type StringRwRegister struct {
	Name     string
	Start    uint16
	Num      uint16
	Parse    func(data []byte) string
	Validate func(value string) error
	Bytes    func(value string, dst []byte)
}

func (s *StringRwRegister) GetName() string {
	return s.Name
}

func (s *StringRwRegister) GetStart() uint16 {
	return s.Start
}

func (s *StringRwRegister) GetNum() uint16 {
	return s.Num
}

func (s *StringRwRegister) Decode(data []byte) interface{} {
	return s.Parse(data)
}

func (s *StringRwRegister) Verify(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: string,实际：%v", reflect.TypeOf(value))
	}
	if s.Validate != nil {
		return s.Validate(str)
	}
	return nil
}

func (s *StringRwRegister) Encode(value interface{}) []byte {
	dst := make([]byte, s.Num*2)
	s.Bytes(value.(string), dst)
	return dst
}

type Uint16RwRegister struct {
	Name     string
	Start    uint16
	Order    binary.ByteOrder
	Parse    func(data uint16) interface{}
	Validate func(value uint16) error
}

func (u *Uint16RwRegister) GetName() string {
	return u.Name
}

func (u *Uint16RwRegister) GetStart() uint16 {
	return u.Start
}

func (u *Uint16RwRegister) GetNum() uint16 {
	return 1
}

func (u *Uint16RwRegister) Decode(data []byte) interface{} {
	var order binary.ByteOrder = binary.BigEndian
	if u.Order != nil {
		order = u.Order
	}
	ui16 := order.Uint16(data)
	if u.Parse != nil {
		return u.Parse(ui16)
	}
	return ui16
}

func (u *Uint16RwRegister) Verify(value interface{}) error {
	f64, ok := value.(float64)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
	}
	// 实际是uint16的数值范围
	if f64 <= 0 || f64 > 65536 {
		return fmt.Errorf("参数范围越界，期望: 0～65536,实际：%v", f64)
	}
	if u.Validate != nil {
		return u.Validate(uint16(f64))
	}
	return nil
}

func (u *Uint16RwRegister) Encode(value interface{}) []byte {
	b := make([]byte, 2)
	var order binary.ByteOrder = binary.BigEndian
	if u.Order != nil {
		order = u.Order
	}
	order.PutUint16(b, uint16(value.(float64)))
	return b
}

type ByteRwRegister struct {
	Name     string
	Start    uint16
	Order    binary.ByteOrder
	Parse    func(data []byte) byte
	Validate func(value byte) error
}

func (b *ByteRwRegister) GetName() string {
	return b.Name
}

func (b *ByteRwRegister) GetStart() uint16 {
	return b.Start
}

func (b *ByteRwRegister) GetNum() uint16 {
	return 1
}

func (b *ByteRwRegister) Decode(data []byte) interface{} {
	return b.Parse(data)
}

func (b *ByteRwRegister) Verify(value interface{}) error {
	f64, ok := value.(float64)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
	}
	// 1个字节的数值范围
	if f64 < 0 || f64 > 255 {
		return fmt.Errorf("参数范围越界，期望: 0～255,实际：%v", f64)
	}
	if b.Validate != nil {
		return b.Validate(byte(f64))
	}
	return nil
}

func (b *ByteRwRegister) Encode(value interface{}) []byte {
	return []byte{byte(value.(float64))}
}

type Float32RwRegister struct {
	Name     string
	Start    uint16
	Parse    func(data float32) byte
	Validate func(value float32) error
}

func (f *Float32RwRegister) GetName() string {
	return f.Name
}

func (f *Float32RwRegister) GetStart() uint16 {
	return f.Start
}

func (f *Float32RwRegister) GetNum() uint16 {
	return 2
}

func (f *Float32RwRegister) Decode(data []byte) interface{} {
	bits := binary.LittleEndian.Uint32(data)
	f32 := math.Float32frombits(bits)
	if f.Parse != nil {
		return f.Parse(f32)
	}
	return f32
}

func (f *Float32RwRegister) Verify(value interface{}) error {
	f64, ok := value.(float64)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
	}
	if f.Validate != nil {
		return f.Validate(float32(f64))
	}
	return nil
}

func (f *Float32RwRegister) Encode(value interface{}) []byte {
	bits := math.Float32bits(float32(value.(float64)))

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, bits)
	return bytes
}
