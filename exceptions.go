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
	// AcknowledgeSlave has accepted request and is processing it, but a long duration of time is required. This response is returned to prevent a timeout error from occurring in the master. Master can next issue a Poll Program Complete message to determine whether processing is completed.
	AcknowledgeSlave Exception = 5
	// SlaveDeviceBusy is engaged in processing a long-duration command. Master should retry later.
	SlaveDeviceBusy Exception = 6
	// NegativeAcknowledge Slave cannot perform the programming functions. Master should request diagnostic or error information from slave.
	NegativeAcknowledge Exception = 7
	// MemoryParityError Slave detected a parity error in memory. Master can retry the request, but service may be required on the slave device.
	MemoryParityError Exception = 8
	// GatewayPathUnavailable Specialized for Modbus gateways. Indicates a misconfigured gateway.
	GatewayPathUnavailable Exception = 10
	// GatewayTargetDeviceFailedtoRespond Specialized for Modbus gateways. Sent when slave fails to respond.
	GatewayTargetDeviceFailedtoRespond Exception = 11
)

func (e Exception) Error() string {
	return fmt.Sprintf("%d", e)
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
	case AcknowledgeSlave:
		str = fmt.Sprintf("AcknowledgeSlave")
	case SlaveDeviceBusy:
		str = fmt.Sprintf("SlaveDeviceBusy")
	case NegativeAcknowledge:
		str = fmt.Sprintf("NegativeAcknowledge")
	case MemoryParityError:
		str = fmt.Sprintf("MemoryParityError")
	case GatewayPathUnavailable:
		str = fmt.Sprintf("GatewayPathUnavailable")
	case GatewayTargetDeviceFailedtoRespond:
		str = fmt.Sprintf("GatewayTargetDeviceFailedtoRespond")
	default:
		str = fmt.Sprintf("unknown")
	}
	return str
}
