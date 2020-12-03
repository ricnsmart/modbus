package modbus

import (
	"testing"
)

func TestCRCModbus(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		args    args
		wantCrc uint16
	}{
		{args: struct{ data []byte }{data: []byte("aabb")}, wantCrc: 0b1000111111100110},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotCrc := CRCModbus(tt.args.data); gotCrc != tt.wantCrc {
				t.Errorf("got %v, want %v", gotCrc, tt.wantCrc)
			}
		})
	}
}
