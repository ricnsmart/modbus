package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type ByteOrder interface {
	BytesToUint16(bytes []byte) []uint16
	Uint16ToBytes(values []uint16) []byte
	BytesToUint32(bytes []byte) []uint32
	Uint32ToBytes(values []uint32) []byte
	BytesToFloat32(bytes []byte) float32
	Float32ToBytes(value float32) []byte
	EncodeUint16(bytes *[]byte, value uint16)
	EncodeUint32(bytes *[]byte, value uint32)
	EncodeFloat32(bytes *[]byte, value float32)
	DecodeUint16s(bytes *[]byte, num uint) (vals []uint16, err error)
	DecodeUint32s(bytes *[]byte, num uint) (vals []uint32, err error)
	DecodeFloat32s(bytes *[]byte, num uint) (vals []float32, err error)
	Uint16ToBits(bs []byte) (r []byte)
	BitsToUint16(src []byte, dst []byte)
}

type (
	bigEndian    struct{}
	littleEndian struct{}
)

// LittleEndian is the little-endian implementation of ByteOrder.
var LittleEndian littleEndian

// BigEndian is the big-endian implementation of ByteOrder.
var BigEndian bigEndian

// BytesToUint16 converts a big endian array of bytes to an array of unit16s
func (bigEndian) BytesToUint16(bytes []byte) []uint16 {
	values := make([]uint16, len(bytes)/2)

	for i := range values {
		values[i] = binary.BigEndian.Uint16(bytes[i*2 : (i+1)*2])
	}
	return values
}

// Uint16ToBytes converts an array of uint16s to a big endian array of bytes
func (bigEndian) Uint16ToBytes(values []uint16) []byte {
	bytes := make([]byte, len(values)*2)

	for i, value := range values {
		binary.BigEndian.PutUint16(bytes[i*2:(i+1)*2], value)
	}
	return bytes
}

// BytesToUint32 converts a big endian array of bytes to an array of unit32s
func (bigEndian) BytesToUint32(bytes []byte) []uint32 {
	values := make([]uint32, len(bytes)/4)

	for i := range values {
		values[i] = binary.BigEndian.Uint32(bytes[i*4 : (i+1)*4])
	}
	return values
}

// Uint32ToBytes converts an array of uint32s to a big endian array of bytes
func (bigEndian) Uint32ToBytes(values []uint32) []byte {
	bytes := make([]byte, len(values)*4)

	for i, value := range values {
		binary.BigEndian.PutUint32(bytes[i*4:(i+1)*4], value)
	}
	return bytes
}

// BytesToFloat32 converts a big endian array of bytes to a float32
func (bigEndian) BytesToFloat32(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)

	return math.Float32frombits(bits)
}

// Float32ToBytes converts a float32 to a big endian array of bytes
func (bigEndian) Float32ToBytes(value float32) []byte {
	bits := math.Float32bits(value)

	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, bits)
	return bytes
}

// Float32sToBytes converts an array of float32 to a big endian array of bytes
func (bigEndian) Float32sToBytes(values []float32) []byte {
	buf := make([]byte, 0)
	for _, value := range values {
		bits := math.Float32bits(value)
		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, bits)
		buf = append(buf, bytes...)
	}

	return buf
}

// EncodeUint16 将一个uint16类型的数字转换为大端的字节充入一个数组的尾部
// 数组前面的内容可以不必是uint16类型
func (bigEndian) EncodeUint16(bytes *[]byte, value uint16) {
	bArr := make([]byte, 2)
	binary.BigEndian.PutUint16(bArr[0:2], value)
	*bytes = append(*bytes, bArr...)
}

// EncodeUint32 将一个uint32类型的数字转换为大端的字节充入一个数组的尾部
func (bigEndian) EncodeUint32(bytes *[]byte, value uint32) {
	bArr := make([]byte, 4)
	binary.BigEndian.PutUint32(bArr[0:4], value)
	*bytes = append(*bytes, bArr...)
}

// EncodeFloat32 将一个float32类型的数字转换为大端的字节充入一个数组的尾部
func (bigEndian) EncodeFloat32(bytes *[]byte, value float32) {
	bArr := BigEndian.Float32ToBytes(value)
	*bytes = append(*bytes, bArr...)
}

// DecodeUint16s 读取字节数组中，指定长度的uint16类型数字，返回一个uint16的数组
// 适用于混乱类型的字节流
func (bigEndian) DecodeUint16s(bytes *[]byte, num uint) (vals []uint16, err error) {
	needLen := (int)(2 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	vals = BigEndian.BytesToUint16((*bytes)[:needLen])
	*bytes = (*bytes)[needLen:]

	return
}

// DecodeUint32s 读取字节数组中，指定长度的uint32类型数字，返回一个uint32的数组
// 适用于混乱类型的字节流
func (bigEndian) DecodeUint32s(bytes *[]byte, num uint) (vals []uint32, err error) {
	needLen := (int)(4 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	vals = BigEndian.BytesToUint32((*bytes)[0:needLen])
	*bytes = (*bytes)[needLen:]

	return
}

// DecodeFloat32s 读取字节数组中，指定长度的float32类型数字，返回一个float32的数组
// 适用于混乱类型的字节流
func (bigEndian) DecodeFloat32s(bytes *[]byte, num uint) (vals []float32, err error) {
	needLen := (int)(4 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	fp32vals := make([]float32, num)

	for i := (uint)(0); i < num; i++ {
		fp32vals[i] = BigEndian.BytesToFloat32((*bytes)[i*4 : (i+1)*4])
	}

	*bytes = (*bytes)[needLen:]

	return fp32vals, nil
}

// BytesToUint16 converts a little endian array of bytes to an array of unit16s
func (littleEndian) BytesToUint16(bytes []byte) []uint16 {
	values := make([]uint16, len(bytes)/2)

	for i := range values {
		values[i] = binary.LittleEndian.Uint16(bytes[i*2 : (i+1)*2])
	}
	return values
}

// Uint16ToBytes converts an array of uint16s to a little endian array of bytes
func (littleEndian) Uint16ToBytes(values []uint16) []byte {
	bytes := make([]byte, len(values)*2)

	for i, value := range values {
		binary.LittleEndian.PutUint16(bytes[i*2:(i+1)*2], value)
	}
	return bytes
}

// BytesToUint32 converts a little endian array of bytes to an array of unit32s
func (littleEndian) BytesToUint32(bytes []byte) []uint32 {
	values := make([]uint32, len(bytes)/4)

	for i := range values {
		values[i] = binary.LittleEndian.Uint32(bytes[i*4 : (i+1)*4])
	}
	return values
}

// Uint32ToBytes converts an array of uint32s to a little endian array of bytes
func (littleEndian) Uint32ToBytes(values []uint32) []byte {
	bytes := make([]byte, len(values)*4)

	for i, value := range values {
		binary.LittleEndian.PutUint32(bytes[i*4:(i+1)*4], value)
	}
	return bytes
}

// BytesToFloat32 converts a little endian array of bytes to a float32
func (littleEndian) BytesToFloat32(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)

	return math.Float32frombits(bits)
}

// Float32ToBytes converts a float32 to a little endian array of bytes
func (littleEndian) Float32ToBytes(value float32) []byte {
	bits := math.Float32bits(value)

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, bits)
	return bytes
}

// Float32sToBytes converts an array of float32 to a little endian array of bytes
func (littleEndian) Float32sToBytes(values []float32) []byte {
	buf := make([]byte, 0)
	for _, value := range values {
		bits := math.Float32bits(value)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, bits)
		buf = append(buf, bytes...)
	}

	return buf
}

func (littleEndian) EncodeUint16(bytes *[]byte, value uint16) {
	bArr := make([]byte, 2)
	binary.LittleEndian.PutUint16(bArr[0:2], value)
	*bytes = append(*bytes, bArr...)
}

func (littleEndian) EncodeUint32(bytes *[]byte, value uint32) {
	bArr := make([]byte, 4)
	binary.LittleEndian.PutUint32(bArr[0:4], value)
	*bytes = append(*bytes, bArr...)
}

func (littleEndian) EncodeFloat32(bytes *[]byte, value float32) {
	bArr := LittleEndian.Float32ToBytes(value)
	*bytes = append(*bytes, bArr...)
}

func (littleEndian) DecodeUint16s(bytes *[]byte, num uint) (vals []uint16, err error) {
	needLen := (int)(2 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	vals = LittleEndian.BytesToUint16((*bytes)[:needLen])
	*bytes = (*bytes)[needLen:]

	return
}

func (littleEndian) DecodeUint32s(bytes *[]byte, num uint) (vals []uint32, err error) {
	needLen := (int)(4 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	vals = LittleEndian.BytesToUint32((*bytes)[0:needLen])
	*bytes = (*bytes)[needLen:]

	return
}

func (littleEndian) DecodeFloat32s(bytes *[]byte, num uint) (vals []float32, err error) {
	needLen := (int)(4 * num)
	if len(*bytes) < needLen {
		err = errors.New("bytes is not Enough")
		return
	}

	fp32vals := make([]float32, num)

	for i := (uint)(0); i < num; i++ {
		fp32vals[i] = LittleEndian.BytesToFloat32((*bytes)[i*4 : (i+1)*4])
	}

	*bytes = (*bytes)[needLen:]

	return fp32vals, nil
}

// BitsToByte 比特转字节
// buf 需要是大端模式排列比特位
func BitsToByte(buf []byte) (u byte) {
	for i := 0; i < len(buf); i++ {
		bit := buf[i]
		if bit != 0 && bit != 1 {
			panic(fmt.Sprintf("bit must 0 or 1,actual: %v", bit))
		}
		u += bit << (i)
	}
	return
}

// ByteToBits 字节转比特
// 大端模式排列比特位
func ByteToBits(d byte) []byte {
	b := make([]byte, 8)
	for i := 0; d > 0; d /= 2 {
		b[i] = d % 2
		i++
	}
	return b
}

// Uint16ToBits uint16转比特
// 大端模式排列比特位
func (bigEndian) Uint16ToBits(bs []byte) (r []byte) {
	_ = bs[1]
	r = append(ByteToBits(bs[1]), ByteToBits(bs[0])...)
	return
}

// Uint16ToBits uint16转比特
// 大端模式排列比特位
func (littleEndian) Uint16ToBits(bs []byte) (r []byte) {
	_ = bs[1]
	r = append(ByteToBits(bs[0]), ByteToBits(bs[1])...)
	return
}

// BitsToUint16 比特转uint16
func (bigEndian) BitsToUint16(src []byte, dst []byte) {
	_ = src[15]
	dst[0] = BitsToByte(src[8:16])
	dst[1] = BitsToByte(src[0:8])
}

// BitsToUint16 比特转uint16
func (littleEndian) BitsToUint16(src []byte, dst []byte) {
	_ = src[15]
	dst[0] = BitsToByte(src[0:8])
	dst[1] = BitsToByte(src[8:16])
}
