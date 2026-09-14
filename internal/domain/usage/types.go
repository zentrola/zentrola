// Package usage 定义真实上游调用 Attempt 的业务语义。
package usage

type Status string
type Scene string

const (
	Success      Status = "SUCCESS"
	Failed       Status = "FAILED"
	Cancelled    Status = "CANCELLED"
	ModelGateway Scene  = "MODEL_GATEWAY"
)

const MVPAttemptNo int64 = 1
