package modbus

import (
	"fmt"
	"reflect"
	"testing"
)

var (
	// 保护开关脱扣
	// 0~1:漏电保护; 00:不分闸;01:延时分闸; 注意这里的01,第一个0代表bit0=0，第二个1代表bit1=1
	// 2~3:温度保护; 00:不分闸;01:延时分闸;
	// 4~5:过/欠压保护; 00:不分闸;01:延时分闸;
	// 6~7:过载保护; 00:不分闸;01:延时分闸;
	// 其他:预留
	// 延时开闸--数值达到报警值后随“保护延时”寄存器时间到达后报警分闸;
	// 默认小端
	ProtectionSwitchTrip = &BitRwRegister{
		Name:  "ProtectionSwitchTrip",
		Start: 0x0200,
		Params: []BitParam{
			{
				Name:     "LeakageProtection",
				Start:    0,
				Len:      2,
				Parse:    getProtectionStatus,
				Validate: validateProtectionStatus,
				Bytes:    transferProtectionStatus,
			},
			{
				Name:     "TemperatureProtection",
				Start:    2,
				Len:      2,
				Parse:    getProtectionStatus,
				Validate: validateProtectionStatus,
				Bytes:    transferProtectionStatus,
			},
			{
				Name:     "VoltageProtection",
				Start:    4,
				Len:      2,
				Parse:    getProtectionStatus,
				Validate: validateProtectionStatus,
				Bytes:    transferProtectionStatus,
			},
			{
				Name:     "OverloadProtection",
				Start:    6,
				Len:      2,
				Parse:    getProtectionStatus,
				Validate: validateProtectionStatus,
				Bytes:    transferProtectionStatus,
			},
		},
	}
)

func TestBitRwRegister_Encode(t *testing.T) {
	tests := []struct {
		params map[string]interface{}
		dst    []byte
	}{
		{
			params: map[string]interface{}{
				"LeakageProtection":     "延时分闸",
				"OverloadProtection":    "延时分闸",
				"TemperatureProtection": "延时分闸",
				"VoltageProtection":     "延时分闸",
			},
			dst: []byte{0x00, 0xaa},
		},
		{
			params: map[string]interface{}{
				"LeakageProtection":     "不分闸",
				"OverloadProtection":    "不分闸",
				"TemperatureProtection": "不分闸",
				"VoltageProtection":     "不分闸",
			},
			dst: []byte{0x00, 0x00},
		},
	}
	for _, tt := range tests {
		dst := make([]byte, 2)
		ProtectionSwitchTrip.Encode(tt.params, dst)
		if !reflect.DeepEqual(dst, tt.dst) {
			t.Errorf("Encode error 期望：%v,实际：%v", tt.dst, dst)
		}
	}
}

func getProtectionStatus(data []byte) interface{} {
	_ = data[1]

	// 2个字节，默认大端模式，高位在前，地位在后，所以取内存高位
	switch {
	case data[0] == 0 && data[1] == 0:
		return "不分闸"
	case data[0] == 0 && data[1] == 1:
		return "延时分闸"
	default:
		panic(fmt.Sprintf("保护状态异常，期望：0,0或0,1，实际：%x", data))
	}
}

func validateProtectionStatus(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("参数类型错误，期望: string,实际：%v", reflect.TypeOf(value))
	}
	if str != "不分闸" && str != "延时分闸" {
		return fmt.Errorf("参数值错误，期望: 不分闸或延时分闸,实际：%v", str)
	}
	return nil
}

func transferProtectionStatus(value interface{}) []byte {
	switch value.(string) {
	case "不分闸":
		return []byte{0, 0}
	case "延时分闸":
		return []byte{0, 1}
	default:
		panic(fmt.Errorf("参数值错误，期望: 不分闸或延时分闸,实际：%v", value))
	}
}
