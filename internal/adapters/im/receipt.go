package im

import "fmt"

// receiptLimit 是 IM 回执读取的字节上限:覆盖合法回执的最坏常见编码
// (sendMessage 正文上限 4096 字符,回执回显全文且非 ASCII 全量转义,另加
// 实体与元数据)并留有余量;超出即停止读取,避免中转端异常大响应造成
// 内存压力。上限与最坏合法回执的关系由
// TestTelegramReceiptLimitCoversWorstLegalReceipt 钉住。
const receiptLimit = 64 << 10

// 回执无法核验的三种原因。
const (
	ReasonReceiptExceedsLimit = "receipt exceeds limit"
	ReasonReceiptReadFailed   = "receipt read failed"
	ReasonReceiptDecodeFailed = "receipt decode failed"
)

// DeliveryUnverifiedError 标记 HTTP 2xx 但回执超限、读取不完整或无法解析:
// 消息可能已送达但协调器无法核实,不按普通失败处理(避免误导性重发)。
// 错误文本只含有界元数据,不携带远端响应正文。
type DeliveryUnverifiedError struct {
	Reason     string
	HTTPStatus int
	Limit      int
	Read       int    // 实际读到的字节数;超限时为上限+1
	Detail     string // 底层原因说明,不含远端正文
}

func (e *DeliveryUnverifiedError) Error() string {
	switch e.Reason {
	case ReasonReceiptExceedsLimit:
		return fmt.Sprintf("receipt exceeds limit %d (http %d, read at least %d)", e.Limit, e.HTTPStatus, e.Read)
	case ReasonReceiptReadFailed:
		return fmt.Sprintf("receipt read failed after %d bytes (http %d): %s", e.Read, e.HTTPStatus, e.Detail)
	default:
		return fmt.Sprintf("receipt decode failed (http %d): %s", e.HTTPStatus, e.Detail)
	}
}
