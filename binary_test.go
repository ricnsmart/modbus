package modbus

import (
	"reflect"
	"testing"
)

func TestToByte(t *testing.T) {
	type args struct {
		buf []byte
	}
	tests := []struct {
		name  string
		args  args
		wantU byte
	}{
		{args: args{buf: []byte{0, 0, 1, 0, 0, 0, 0, 0}}, wantU: byte(4)},
		{args: args{buf: []byte{1, 1, 1, 0, 0, 0, 0, 0}}, wantU: byte(7)},
		{args: args{buf: []byte{0, 0, 0, 1, 0, 0, 0, 0}}, wantU: byte(8)},
		{args: args{buf: []byte{0, 0, 0, 0, 1, 0, 0, 0}}, wantU: byte(16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotU := BitsToByte(tt.args.buf); gotU != tt.wantU {
				t.Errorf("BinaryToInt() = %v, want %v", gotU, tt.wantU)
			}
		})
	}
}

func Test_bigEndian_Uint16ToBinaries(t *testing.T) {
	type args struct {
		bs []byte
	}
	tests := []struct {
		name  string
		args  args
		wantR []byte
	}{
		{args: args{bs: []byte{128, 16}}, wantR: []byte{0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
		{args: args{bs: []byte{4, 0}}, wantR: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bi := bigEndian{}
			if gotR := bi.Uint16ToBits(tt.args.bs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("Uint16ToBits() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func Test_littleEndian_Uint16ToBinaries(t *testing.T) {
	type args struct {
		bs []byte
	}
	tests := []struct {
		name  string
		args  args
		wantR []byte
	}{
		{name: "1", args: args{bs: []byte{128, 16}}, wantR: []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0}},
		{name: "2", args: args{bs: []byte{4, 0}}, wantR: []byte{0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			li := littleEndian{}
			if gotR := li.Uint16ToBits(tt.args.bs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("Uint16ToBits() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func Test_bigEndian_BitsToUint16(t *testing.T) {
	type args struct {
		src []byte
		dst []byte
	}
	tests := []struct {
		name  string
		args  args
		wantR []byte
	}{
		{args: args{src: []byte{0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}}, wantR: []byte{128, 16}},
		{args: args{src: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}}, wantR: []byte{4, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bi := bigEndian{}
			dst := make([]byte, 2)
			bi.BitsToUint16(tt.args.src, dst)
			if !reflect.DeepEqual(dst, tt.wantR) {
				t.Errorf("Uint16ToBits() = %v, want %v", dst, tt.wantR)
			}
		})
	}
}

func Test_littleEndian_BitsToUint16(t *testing.T) {
	type args struct {
		src []byte
		dst []byte
	}
	tests := []struct {
		name  string
		args  args
		wantR []byte
	}{
		{name: "1", args: args{src: []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0}}, wantR: []byte{128, 16}},
		{name: "2", args: args{src: []byte{0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, wantR: []byte{4, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			li := littleEndian{}
			dst := make([]byte, 2)
			li.BitsToUint16(tt.args.src, dst)
			if !reflect.DeepEqual(dst, tt.wantR) {
				t.Errorf("Uint16ToBits() = %v, want %v", dst, tt.wantR)
			}
		})
	}
}
