package modbus

type Nameable interface {
	Name() string
}

// Register 寄存器
// 1个寄存器包含两个字节
type Register interface {
	// Start 寄存器起始地址
	Start() uint16
	// Num 寄存器数量
	Num() uint16
}

type Readable interface {
	Decode(data []byte, values map[string]any)
}

type Writable interface {
	Encode(params map[string]any, dst []byte) error
}

type ReadableRegister interface {
	Register
	Readable
}

func DecodeRegisters[T ReadableRegister](rs []T, data []byte, values map[string]any) {
	start, _ := FindStartAndNum(rs)
	for _, r := range rs {
		start := (r.Start() - start) * 2
		end := start + r.Num()*2
		r.Decode(data[start:end], values)
	}
}

type ReadableRegisters struct {
	// 首个寄存器起始地址，用于确定内部寄存器位置
	// 读取到的寄存器数据起始地址并不一定等于registers[0].Start()
	// 因此需要start参数
	start uint16
	// 寄存器数量；*2即为字节长度
	num uint16

	registers []ReadableRegister
}

func NewReadableRegisters(start uint16, num uint16, registers []ReadableRegister) *ReadableRegisters {
	return &ReadableRegisters{start: start, num: num, registers: registers}
}

func (p *ReadableRegisters) Start() uint16 {
	return p.start
}

func (p *ReadableRegisters) Num() uint16 {
	return p.num
}

func (p *ReadableRegisters) Decode(data []byte, values map[string]any) {
	for _, r := range p.registers {
		start := (r.Start() - p.start) * 2
		end := start + r.Num()*2
		r.Decode(data[start:end], values)
	}
}

type WritableRegister interface {
	Register
	Writable
}

type ReadWriteRegister interface {
	Register
	Readable
	Writable
}

func NewReadRTUFrameWithRegisters[T ReadableRegister](address uint8, rs []T) *RTUFrame {
	start, num := FindStartAndNum(rs)
	f := &RTUFrame{Address: address, Function: 0x03}
	SetDataWithRegisterAndNumber(f, start, num)
	return f
}

func NewReadRTUFrame[T ReadableRegister](address uint8, r T) *RTUFrame {
	f := &RTUFrame{Address: address, Function: 0x03}
	SetDataWithRegisterAndNumber(f, r.Start(), r.Num())
	return f
}

// NewWriteRTUFrameWithRegisters
// 多个可写入的寄存器合成一个用于写入的RTU帧数据
func NewWriteRTUFrameWithRegisters[T WritableRegister](address uint8, rs []T, params map[string]any) (*RTUFrame, error) {
	start, num := FindStartAndNum(rs)
	body := make([]byte, num*2)
	for _, r := range rs {
		start := (r.Start() - start) * 2
		end := start + r.Num()*2
		if err := r.Encode(params, body[start:end]); err != nil {
			return nil, err
		}
	}
	f := &RTUFrame{Address: address, Function: 0x10}

	SetDataWithRegisterAndNumberAndBytes(f, start, num, body)
	return f, nil
}

// NewWriteRTUFrame 单个寄存器生成写入RTUFrame
func NewWriteRTUFrame[T WritableRegister](address uint8, w T, params map[string]any) (*RTUFrame, error) {
	f := &RTUFrame{Address: address, Function: 0x10}
	body := make([]byte, w.Num()*2)
	if err := w.Encode(params, body); err != nil {
		return nil, err
	}
	SetDataWithRegisterAndNumberAndBytes(f, w.Start(), w.Num(), body)
	return f, nil
}

func FindStartAndNum[T Register](rs []T) (start, num uint16) {
	first := rs[0]
	var end = rs[0]

	for i := 0; i < len(rs)-1; i++ {
		if first.Start() > rs[i+1].Start() {
			first = rs[i+1]
		}

		if end.Start() < rs[i+1].Start() {
			end = rs[i+1]
		}
	}

	start = first.Start()

	num = end.Start() - start + 1 + end.Num()

	return
}

type NameableReadWriteRegister interface {
	Nameable
	ReadWriteRegister
}

func Names[T Nameable](rs []T) []string {
	var names []string
	for _, r := range rs {
		names = append(names, r.Name())
	}
	return names
}

type MultiParamReadWriteRegister interface {
	Names() []string
	ReadWriteRegister
}
