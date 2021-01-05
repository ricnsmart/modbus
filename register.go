package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
)

// MODBUS数据域采用“BIG INDIAN”模式，即高位字节在前低位字节在后。例如:
// 1个16位寄存器包含数值为0x12AB 寄存器数值发送顺序为:
// 高位字节= 0x12
// 低位字节= 0xAB
// 但有时因为厂商自定义协议的关系，可能需要使用小断
// 这时修改寄存器Order（binary.ByteOrder）参数即可
type Register interface {
	GetName() string
	// 获取寄存器地址
	GetStart() uint16
	// 获取寄存器数量
	GetNum() uint16
}

type Readable interface {
	// 为了适应单个寄存器，两个字节代表2个参数的情况和
	// 单个寄存器，每个比特代表不同参数的情况
	// 所以使用了result去获取decode结果
	Decode(data []byte, results map[string]interface{})
}

type Writable interface {
	// 为了适应单个寄存器，两个字节代表2个参数的情况和
	// 单个寄存器，每个比特代表不同参数的情况
	// 所以使用map来作为输入值，让寄存器自行取用输入值
	Encode(params map[string]interface{}, dst []byte) error
}

type ReadableRegister interface {
	Register
	Readable
}

// 生成读取单个寄存器的报文
func NewReadPacket(address uint8, r ReadableRegister) []byte {
	f := &RTUFrame{Address: address, Function: Read}
	SetDataWithRegisterAndNumber(f, r.GetStart(), r.GetNum())
	return f.Bytes()
}

// 必须是相邻的寄存器，中间可以出现预留空白寄存器
type ReadableRegisters []ReadableRegister

// 将可读寄存器转换为读取寄存器标准modbus报文
func (rs ReadableRegisters) NewReadPacket(address uint8) []byte {
	f := &RTUFrame{Address: address, Function: Read}
	SetDataWithRegisterAndNumber(f, rs.GetStart(), rs.GetNum())
	return f.Bytes()
}

func (rs ReadableRegisters) GetStart() uint16 {
	return rs[0].GetStart()
}

func (rs ReadableRegisters) GetNum() uint16 {
	// 最后一个寄存器-最开始的寄存器 = 总共的寄存器数量
	// 这种算法可以包括 即使中间有预留寄存器的情况
	last := rs[len(rs)-1]
	lastRegister := last.GetStart() + last.GetNum() - 1
	return lastRegister - rs[0].GetStart() + 1
}

// 必须是连续的寄存器才能读取
func (rs ReadableRegisters) Decode(data []byte) map[string]interface{} {
	m := make(map[string]interface{})
	for _, r := range rs {
		start := (r.GetStart() - rs.GetStart()) * 2
		end := start + r.GetNum()*2
		r.Decode(data[start:end], m)
	}
	return m
}

type Float32RoRegister struct {
	Name  string
	Start uint16
	Num   uint16
	Order binary.ByteOrder
	Get   func(value float32) interface{}
}

func (f *Float32RoRegister) GetStart() uint16 {
	return f.Start
}

func (f *Float32RoRegister) GetNum() uint16 {
	return f.Num
}

func (f *Float32RoRegister) Decode(data []byte, results map[string]interface{}) {
	if f.Order == nil {
		f.Order = binary.BigEndian
	}
	bits := f.Order.Uint32(data)
	f32 := math.Float32frombits(bits)
	if f.Get != nil {
		results[f.Name] = f.Get(f32)
	} else {
		results[f.Name] = f32
	}
}

type Uint16RoRegister struct {
	Name  string
	Start uint16
	Order binary.ByteOrder
	Get   func(value uint16) interface{}
}

func (u *Uint16RoRegister) GetName() string {
	return u.Name
}

func (u *Uint16RoRegister) GetStart() uint16 {
	return u.Start
}

func (u *Uint16RoRegister) GetNum() uint16 {
	return 1
}

func (u *Uint16RoRegister) Decode(data []byte, results map[string]interface{}) {
	if u.Order == nil {
		u.Order = binary.BigEndian
	}
	ui16 := u.Order.Uint16(data)
	if u.Get != nil {
		results[u.Name] = u.Get(ui16)
	} else {
		results[u.Name] = ui16
	}
}

type ByteRoParam struct {
	Name string
	Get  func(data byte) interface{}
}

// 单个寄存器，两个字节代表两个参数
// 注意：Params长度必须<=2
type DoubleParamRoRegister struct {
	Name   string
	Start  uint16
	Params []ByteRoParam
}

func (b *DoubleParamRoRegister) GetName() string {
	return b.Name
}

func (b *DoubleParamRoRegister) GetStart() uint16 {
	return b.Start
}

func (b *DoubleParamRoRegister) GetNum() uint16 {
	return 1
}

func (b *DoubleParamRoRegister) Decode(data []byte, results map[string]interface{}) {
	if len(b.Params) > 2 {
		panic(fmt.Sprintf("DoubleParamRwRegister must have less than 2 params,actual: %v", len(b.Params)))
	}
	// 边界检查
	_ = data[1]
	for index, p := range b.Params {
		if p.Get == nil {
			results[p.Name] = data[index]
		} else {
			results[p.Name] = p.Get(data[index])
		}
	}
}

// 由调用方去处理大小端问题
type StringRoRegister struct {
	Name  string
	Start uint16
	Num   uint16
	Get   func(data []byte) string
}

func (s *StringRoRegister) GetName() string {
	return s.Name
}

func (s *StringRoRegister) GetStart() uint16 {
	return s.Start
}

func (s *StringRoRegister) GetNum() uint16 {
	return s.Num
}

func (s *StringRoRegister) Decode(data []byte, results map[string]interface{}) {
	if s.Get == nil {
		panic("StringRwRegister类型必须申明Get方法")
	}
	results[s.Name] = s.Get(data)
}

// 一个寄存器，2个字节，16个比特, 其中包含了多个参数
// Params长度小于16
type BitRoRegister struct {
	Name   string
	Start  uint16
	Order  binary.ByteOrder
	Params []BitRoParam
}

type BitRoParam struct {
	Name  string
	Start uint8
	Len   uint8
	Get   func(data []byte) interface{}
}

func (b *BitRoRegister) GetName() string {
	return b.Name
}

func (b *BitRoRegister) GetStart() uint16 {
	return b.Start
}

func (b *BitRoRegister) GetNum() uint16 {
	return 1
}

func (b *BitRoRegister) Decode(data []byte, results map[string]interface{}) {
	if len(b.Params) > 16 {
		panic(fmt.Sprintf("BitRwRegister must have less than 16 params,actual: %v", len(b.Params)))
	}
	if b.Order == nil {
		b.Order = binary.BigEndian
	}
	u16 := b.Order.Uint16(data)

	for _, p := range b.Params {
		name := p.Name

		buf := make([]byte, p.Len)

		for k := uint8(0); k < p.Len; k++ {
			index := p.Start + k
			bit := u16 >> uint(index) & 1
			buf[k] = byte(bit)
		}

		if p.Get == nil {
			panic("BitRwRegister类型必须申明Get方法")
		}

		results[name] = p.Get(buf)
	}
}

type WritableRegister interface {
	Register
	Writable
}

// 必须是相邻的寄存器，中间可以出现预留空白寄存器
type WritableRegisters []WritableRegister

func (ws WritableRegisters) Encode(params map[string]interface{}) ([]byte, error) {
	result := make([]byte, ws.GetNum()*2)
	for _, w := range ws {
		// 相对位置
		start := (w.GetStart() - ws.GetStart()) * 2
		end := start + w.GetNum()*2
		if err := w.Encode(params, result[start:end]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (ws WritableRegisters) GetStart() uint16 {
	return ws[0].GetStart()
}

func (ws WritableRegisters) GetNum() uint16 {
	last := ws[len(ws)-1]
	lastRegister := last.GetStart() + last.GetNum() - 1
	return lastRegister - ws[0].GetStart() + 1
}

// 将可写寄存器转换为写入寄存器标准modbus报文
func (ws WritableRegisters) NewWritePacket(address uint8, params map[string]interface{}) ([]byte, error) {
	f := &RTUFrame{Address: address, Function: Write}
	buf, err := ws.Encode(params)
	if err != nil {
		return nil, err
	}
	SetDataWithRegisterAndNumberAndBytes(f, ws.GetStart(), ws.GetNum(), buf)
	return f.Bytes(), nil
}

type ReadableAndWritableRegister interface {
	Register
	Readable
	Writable
}

func NewWritePacket(address uint8, w WritableRegister, params map[string]interface{}) ([]byte, error) {
	f := &RTUFrame{Address: address, Function: Write}
	dst := make([]byte, w.GetNum()*2)
	if err := w.Encode(params, dst); err != nil {
		return nil, err
	}
	SetDataWithRegisterAndNumberAndBytes(f, w.GetStart(), w.GetNum(), dst)
	return f.Bytes(), nil
}

// 必须是相邻的寄存器，中间可以出现预留空白寄存器
type ReadableAndWritableRegisters []ReadableAndWritableRegister

func (rws ReadableAndWritableRegisters) GetStart() uint16 {
	return rws[0].GetStart()
}

func (rws ReadableAndWritableRegisters) GetNum() uint16 {
	last := rws[len(rws)-1]
	lastRegister := last.GetStart() + last.GetNum() - 1
	return lastRegister - rws[0].GetStart() + 1
}

// 必须是连续的寄存器才能读取
func (rws ReadableAndWritableRegisters) NewReadPacket(address uint8) []byte {
	f := &RTUFrame{Address: address, Function: Read}
	SetDataWithRegisterAndNumber(f, rws.GetStart(), rws.GetNum())
	return f.Bytes()
}

func (rws ReadableAndWritableRegisters) NewWritePacket(address uint8, params map[string]interface{}) ([]byte, error) {
	f := &RTUFrame{Address: address, Function: Write}
	buf, err := rws.Encode(params)
	if err != nil {
		return nil, err
	}
	SetDataWithRegisterAndNumberAndBytes(f, rws.GetStart(), rws.GetNum(), buf)
	return f.Bytes(), nil
}

func (rws ReadableAndWritableRegisters) Decode(data []byte) map[string]interface{} {
	m := make(map[string]interface{})
	for _, r := range rws {
		start := (r.GetStart() - rws.GetStart()) * 2
		end := start + r.GetNum()*2
		r.Decode(data[start:end], m)
	}
	return m
}

func (rws ReadableAndWritableRegisters) Encode(params map[string]interface{}) ([]byte, error) {
	result := make([]byte, rws.GetNum()*2)
	for _, w := range rws {
		// 相对位置
		start := (w.GetStart() - rws.GetStart()) * 2
		end := start + w.GetNum()*2
		if err := w.Encode(params, result[start:end]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// 由调用方去处理大小端问题
type StringRwRegister struct {
	Name  string
	Start uint16
	Num   uint16
	Get   func(data []byte) string
	Set   func(value string, dst []byte) error
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

func (s *StringRwRegister) Decode(data []byte, results map[string]interface{}) {
	if s.Get == nil {
		panic("StringRwRegister类型必须申明Get方法")
	}
	results[s.Name] = s.Get(data)
}

func (s *StringRwRegister) Encode(params map[string]interface{}, dst []byte) error {
	value, ok := params[s.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", s.Name)
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: string,实际：%v", reflect.TypeOf(value))
	}
	if s.Set == nil {
		panic("StringRwRegister类型必须申明Set方法")
	}
	return s.Set(str, dst)
}

type Uint16RwRegister struct {
	Name     string
	Start    uint16
	Order    binary.ByteOrder
	Get      func(value uint16) interface{}
	Set      func(value interface{}) (uint16, error)
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

func (u *Uint16RwRegister) Decode(data []byte, results map[string]interface{}) {
	if u.Order == nil {
		u.Order = binary.BigEndian
	}
	ui16 := u.Order.Uint16(data)
	if u.Get != nil {
		results[u.Name] = u.Get(ui16)
	} else {
		results[u.Name] = ui16
	}
}

func (u *Uint16RwRegister) Encode(params map[string]interface{}, dst []byte) error {
	value, ok := params[u.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", u.Name)
	}
	var u16 uint16
	if u.Set == nil {
		f64, ok := value.(float64)
		if !ok {
			return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
		}
		// 实际是uint16的数值范围
		if f64 <= 0 || f64 > 65536 {
			return fmt.Errorf("参数范围越界，期望: 0～65536,实际：%v", f64)
		}
		u16 = uint16(f64)

		if u.Validate != nil {
			if err := u.Validate(u16); err != nil {
				return err
			}
		}
	} else {
		result, err := u.Set(value)
		if err != nil {
			return err
		}
		u16 = result
	}
	if u.Order == nil {
		u.Order = binary.BigEndian
	}
	u.Order.PutUint16(dst, u16)
	return nil
}

type ByteRwParam struct {
	Name string
	Get  func(data byte) interface{}
	Set  func(value interface{}) (byte, error)
}

// 单个寄存器，两个字节代表两个参数
// 注意：Params长度必须<=2
type DoubleParamRwRegister struct {
	Name   string
	Start  uint16
	Params []ByteRwParam
}

func (b *DoubleParamRwRegister) GetName() string {
	return b.Name
}

func (b *DoubleParamRwRegister) GetStart() uint16 {
	return b.Start
}

func (b *DoubleParamRwRegister) GetNum() uint16 {
	return 1
}

func (b *DoubleParamRwRegister) Decode(data []byte, results map[string]interface{}) {
	if len(b.Params) > 2 {
		panic(fmt.Sprintf("DoubleParamRwRegister must have less than 2 params,actual: %v", len(b.Params)))
	}
	// 边界检查
	_ = data[1]
	for index, p := range b.Params {
		if p.Get == nil {
			results[p.Name] = data[index]
		} else {
			results[p.Name] = p.Get(data[index])
		}
	}
}

func (b *DoubleParamRwRegister) Encode(params map[string]interface{}, dst []byte) error {
	if len(b.Params) > 2 {
		panic("DoubleParamRwRegister must have less than 2 params")
	}

	for index, p := range b.Params {
		name := p.Name
		value, ok := params[name]
		if !ok {
			return fmt.Errorf("参数 %v 缺失", name)
		}

		if p.Set == nil {
			f64, ok := value.(float64)
			if !ok {
				return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
			}
			if f64 < 0 || f64 > 255 {
				return fmt.Errorf("参数值错误，期望: 0～255,实际：%v", f64)
			}
			dst[index] = byte(f64)
		} else {
			var err error
			dst[index], err = p.Set(value)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type Float32RwRegister struct {
	Name     string
	Start    uint16
	Order    binary.ByteOrder
	Get      func(data float32) interface{}
	Set      func(value interface{}) (float32, error)
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

func (f *Float32RwRegister) Decode(data []byte, results map[string]interface{}) {
	if f.Order == nil {
		f.Order = binary.BigEndian
	}
	bits := f.Order.Uint32(data)
	f32 := math.Float32frombits(bits)
	if f.Get != nil {
		results[f.Name] = f.Get(f32)
	} else {
		results[f.Name] = f32
	}
}

func (f *Float32RwRegister) Verify(params map[string]interface{}) error {
	value, ok := params[f.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", f.Name)
	}
	f64, ok := value.(float64)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
	}
	if f.Validate != nil {
		return f.Validate(float32(f64))
	}
	return nil
}

func (f *Float32RwRegister) Encode(params map[string]interface{}, dst []byte) error {
	value, ok := params[f.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", f.Name)
	}
	var f32 float32
	if f.Set == nil {
		f64, ok := value.(float64)
		if !ok {
			return fmt.Errorf("参数类型错误，期望: float64,实际：%v", reflect.TypeOf(value))
		}
		f32 = float32(f64)
		if f.Validate != nil {
			if err := f.Validate(f32); err != nil {
				return err
			}
		}

	} else {
		var err error
		f32, err = f.Set(value)
		if err != nil {
			return err
		}
	}
	bits := math.Float32bits(f32)
	if f.Order == nil {
		f.Order = binary.BigEndian
	}
	f.Order.PutUint32(dst, bits)
	return nil
}

// 一个寄存器，2个字节，16个比特, 其中包含了多个参数
// Params长度小于16
type BitRwRegister struct {
	Name   string
	Start  uint16
	Order  binary.ByteOrder
	Params []BitRwParam
}

type BitRwParam struct {
	Name  string
	Start uint8
	Len   uint8
	Get   func(data []byte) interface{}
	Set   func(value interface{}) ([]byte, error)
}

func (b *BitRwRegister) GetStart() uint16 {
	return b.Start
}

func (b *BitRwRegister) GetNum() uint16 {
	return 1
}

func (b *BitRwRegister) Decode(data []byte, results map[string]interface{}) {
	if len(b.Params) > 16 {
		panic(fmt.Sprintf("BitRwRegister must have less than 16 params,actual: %v", len(b.Params)))
	}
	if b.Order == nil {
		b.Order = binary.BigEndian
	}
	u16 := b.Order.Uint16(data)

	for _, p := range b.Params {
		name := p.Name

		buf := make([]byte, p.Len)

		for k := uint8(0); k < p.Len; k++ {
			index := p.Start + k
			bit := u16 >> uint(index) & 1
			buf[k] = byte(bit)
		}

		if p.Get == nil {
			panic("BitRwRegister类型必须申明Get方法")
		}

		results[name] = p.Get(buf)
	}
}

func (b *BitRwRegister) GetName() string {
	return b.Name
}

func (b *BitRwRegister) Encode(params map[string]interface{}, dst []byte) error {
	u16 := uint16(0)
	for _, p := range b.Params {
		name := p.Name
		value, ok := params[name]
		if !ok {
			return fmt.Errorf("参数 %v 缺失", name)
		}
		if p.Set == nil {
			panic("BitRwRegister类型必须申明Set方法")
		}
		buf, err := p.Set(value)
		if err != nil {
			return err
		}
		for k, v := range buf {
			index := p.Start + uint8(k)
			u16 = u16 | uint16(v<<index)
		}
	}
	if b.Order == nil {
		b.Order = binary.BigEndian
	}
	b.Order.PutUint16(dst, u16)
	return nil
}
