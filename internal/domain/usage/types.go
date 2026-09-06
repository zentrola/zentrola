// Package usage 定义请求事实与上游 Attempt 的业务语义。
package usage

type Status string
type Scene string
type BillingUnit string

const (
	Success      Status      = "SUCCESS"
	Failed       Status      = "FAILED"
	Cancelled    Status      = "CANCELLED"
	ModelGateway Scene       = "MODEL_GATEWAY"
	Token        BillingUnit = "TOKEN"
	Call         BillingUnit = "CALL"
	Image        BillingUnit = "IMAGE"
	Second       BillingUnit = "SECOND"
)

const MVPAttemptNo int64 = 1
