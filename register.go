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
	GetName() string
	// 为了适应单个寄存器，两个字节代表2个参数的情况和
	// 单个寄存器，每个比特代表不同参数的情况
	// 所以使用map来作为输入值，让寄存器自行取用输入值
	Verify(params map[string]interface{}) error
	Encode(params map[string]interface{}, dst []byte)
}

type ReadableRegister interface {
	Register
	Readable
}

// 必须是相邻的寄存器，中间可以出现预留空白寄存器
type ReadableRegisters []ReadableRegister

// 将可读寄存器转换为读取寄存器标准modbus报文
func (rs ReadableRegisters) ReadBytes(address uint8) []byte {
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
	Parse func(data float32) interface{}
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
	if f.Parse != nil {
		results[f.Name] = f.Parse(f32)
	} else {
		results[f.Name] = f32
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
		if err := w.Verify(params); err != nil {
			return nil, err
		}
		// 相对位置
		start := (w.GetStart() - ws.GetStart()) * 2
		end := start + w.GetNum()*2
		w.Encode(params, result[start:end])
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
func (ws WritableRegisters) WriteBytes(address uint8, params map[string]interface{}) ([]byte, error) {
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
func (rws ReadableAndWritableRegisters) ReadBytes(address uint8) []byte {
	f := &RTUFrame{Address: address, Function: Read}
	SetDataWithRegisterAndNumber(f, rws.GetStart(), rws.GetNum())
	return f.Bytes()
}

func (rws ReadableAndWritableRegisters) WriteBytes(address uint8, params map[string]interface{}) ([]byte, error) {
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
		if err := w.Verify(params); err != nil {
			return nil, err
		}
		// 相对位置
		start := (w.GetStart() - rws.GetStart()) * 2
		end := start + w.GetNum()*2
		w.Encode(params, result[start:end])
	}
	return result, nil
}

// 由调用方去处理大小端问题
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

func (s *StringRwRegister) Decode(data []byte, results map[string]interface{}) {
	results[s.Name] = s.Parse(data)
}

func (s *StringRwRegister) Verify(params map[string]interface{}) error {
	value, ok := params[s.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", s.Name)
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: string,实际：%v", reflect.TypeOf(value))
	}
	if s.Validate != nil {
		return s.Validate(str)
	}
	return nil
}

func (s *StringRwRegister) Encode(params map[string]interface{}, dst []byte) {
	value := params[s.Name]
	if s.Bytes == nil {
		panic("StringRwRegister类型必须申明Bytes()方法")
	}
	s.Bytes(value.(string), dst)
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

func (u *Uint16RwRegister) Decode(data []byte, results map[string]interface{}) {
	if u.Order == nil {
		u.Order = binary.BigEndian
	}
	ui16 := u.Order.Uint16(data)
	if u.Parse != nil {
		results[u.Name] = u.Parse(ui16)
	} else {
		results[u.Name] = ui16
	}
}

func (u *Uint16RwRegister) Verify(params map[string]interface{}) error {
	value, ok := params[u.Name]
	if !ok {
		return fmt.Errorf("参数 %v 缺失", u.Name)
	}
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

func (u *Uint16RwRegister) Encode(params map[string]interface{}, dst []byte) {
	value := params[u.Name]
	if u.Order == nil {
		u.Order = binary.BigEndian
	}
	u.Order.PutUint16(dst, uint16(value.(float64)))
}

type Param struct {
	Name     string
	Validate func(value byte) error
}

// 单个寄存器，两个字节代表两个参数
// 注意：Params长度必须<=2
type DoubleParamRwRegister struct {
	Name   string
	Start  uint16
	Params []Param
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
		results[p.Name] = data[index]
	}
}

func (b *DoubleParamRwRegister) Verify(params map[string]interface{}) error {
	if len(b.Params) > 2 {
		panic("DoubleParamRwRegister must have less than 2 params")
	}
	for _, p := range b.Params {
		name := p.Name
		value, ok := params[name]
		if !ok {
			return fmt.Errorf("参数 %v 缺失", name)
		}

		f64, ok := value.(float64)
		if !ok {
			return fmt.Errorf("参数 %v 类型错误，期望: float64,实际：%v", name, reflect.TypeOf(value))
		}
		// 1个字节的数值范围
		if f64 < 0 || f64 > 255 {
			return fmt.Errorf("参数 %v  范围越界，期望: 0～255,实际：%v", name, f64)
		}
		if p.Validate != nil {
			return p.Validate(byte(value.(float64)))
		}
	}

	return nil
}

func (b *DoubleParamRwRegister) Encode(params map[string]interface{}, dst []byte) {
	for index, p := range b.Params {
		dst[index] = byte(params[p.Name].(float64))
	}
}

type Float32RwRegister struct {
	Name     string
	Start    uint16
	Order    binary.ByteOrder
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

func (f *Float32RwRegister) Decode(data []byte, results map[string]interface{}) {
	if f.Order == nil {
		f.Order = binary.BigEndian
	}
	bits := f.Order.Uint32(data)
	f32 := math.Float32frombits(bits)
	if f.Parse != nil {
		results[f.Name] = f.Parse(f32)
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

func (f *Float32RwRegister) Encode(params map[string]interface{}, dst []byte) {
	value := params[f.Name]
	bits := math.Float32bits(float32(value.(float64)))
	if f.Order == nil {
		f.Order = binary.BigEndian
	}
	f.Order.PutUint32(dst, bits)
}

// 一个寄存器，2个字节，16个比特, 其中包含了多个参数
// Params长度小于16
type BitRwRegister struct {
	Name   string
	Start  uint16
	Params []BitParam
}

type BitParam struct {
	Name     string
	Start    uint8
	Len      uint8
	Parse    func(data []byte) interface{}
	Validate func(value interface{}) error
	Bytes    func(value interface{}, dst []byte)
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
	for _, p := range b.Params {
		name := p.Name

		buf := make([]byte, p.Len)

		for k := uint8(0); k < p.Len; k++ {
			i := p.Start + k
			j := 0
			if i >= 8 {
				i = i - 8
				j = 1
			}

			bit := data[j] >> uint(i) & 1
			buf = append(buf, bit)
		}

		if p.Parse == nil {
			panic("BitRwRegister类型必须申明Parse()方法")
		}

		results[name] = p.Parse(data)
	}
}

func (b *BitRwRegister) GetName() string {
	return b.Name
}

func (b *BitRwRegister) Verify(params map[string]interface{}) error {
	for _, p := range b.Params {
		name := p.Name
		value, ok := params[name]
		if !ok {
			return fmt.Errorf("参数 %v 缺失", name)
		}
		if p.Validate == nil {
			panic("BitRwRegister类型必须申明Validate()方法")
		}
		if err := p.Validate(value); err != nil {
			return err
		}
		continue
	}
	return nil
}

func (b *BitRwRegister) Encode(params map[string]interface{}, dst []byte) {
	for _, p := range b.Params {
		name := p.Name
		value := params[name]
		if p.Bytes == nil {
			panic("BitRwRegister类型必须申明Bytes()方法")
		}
		p.Bytes(value, dst)
	}
}
