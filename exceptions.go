package modbus

import "fmt"

// Exception codes.
type Exception uint8

var (
	// Success operation successful.
	Success Exception
	// IllegalFunction function code received in the query is not recognized or allowed by slave.
	IllegalFunction Exception = 1
	// IllegalDataAddress data address of some or all the required entities are not allowed or do not exist in slave.
	IllegalDataAddress Exception = 2
	// IllegalDataValue value is not accepted by slave.
	IllegalDataValue Exception = 3
	// SlaveDeviceFailure Unrecoverable error occurred while slave was attempting to perform requested action.
	SlaveDeviceFailure Exception = 4
)

func (e Exception) Error() string {
	return e.String()
}

func (e Exception) String() string {
	var str string
	switch e {
	case Success:
		str = fmt.Sprintf("成功")
	case IllegalFunction:
		str = fmt.Sprintf("从站接收到不支持的功能码!")
	case IllegalDataAddress:
		str = fmt.Sprintf("接收到无效的数据地址或者是 请求寄存器不在有效的寄存器范围内")
	case IllegalDataValue:
		str = fmt.Sprintf("读写数据时寄存器数量超出允许范围或字节数!=寄存器数*2;必须连续写的寄存器写入不完整")
	case SlaveDeviceFailure:
		str = fmt.Sprintf("遥控操作失败，或异常码 02、03 规定之外的 情况")
	default:
		str = fmt.Sprintf("未知错误")
	}
	return str
}
