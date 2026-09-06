// Package operation 定义管理操作日志的稳定编码；敏感字段不能进入日志快照。
package operation

type Type string
type OperatorType string
type Result string

const (
	Admin   OperatorType = "ADMIN"
	System  OperatorType = "SYSTEM"
	Success Result       = "SUCCESS"
	Failed  Result       = "FAILED"
)

const (
	AdminInitialize          Type = "ADMIN_INITIALIZE"
	AdminPasswordReset       Type = "ADMIN_PASSWORD_RESET"
	MemberCreate             Type = "MEMBER_CREATE"
	MemberDelete             Type = "MEMBER_DELETE"
	MemberStatusChange       Type = "MEMBER_STATUS_CHANGE"
	AccessKeyCreate          Type = "ACCESS_KEY_CREATE"
	AccessKeyRevoke          Type = "ACCESS_KEY_REVOKE"
	GroupCreate              Type = "GROUP_CREATE"
	GroupMemberAdd           Type = "GROUP_MEMBER_ADD"
	GroupMemberRemove        Type = "GROUP_MEMBER_REMOVE"
	GroupModelGrant          Type = "GROUP_MODEL_GRANT"
	GroupModelRevoke         Type = "GROUP_MODEL_REVOKE"
	ModelStatusChange        Type = "MODEL_STATUS_CHANGE"
	ModelCreate              Type = "MODEL_CREATE"
	ModelUpdate              Type = "MODEL_UPDATE"
	ResourceCreate           Type = "RESOURCE_CREATE"
	ResourceCredentialUpdate Type = "RESOURCE_CREDENTIAL_UPDATE"
	ResourceStatusChange     Type = "RESOURCE_STATUS_CHANGE"
	ResourceConnectionTest   Type = "RESOURCE_CONNECTION_TEST"
	LoginSuccess             Type = "LOGIN_SUCCESS"
	LoginFailed              Type = "LOGIN_FAILED"
	LoginLocked              Type = "LOGIN_LOCKED"
)
