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
	AdminInitialize           Type = "ADMIN_INITIALIZE"
	AdminPasswordReset        Type = "ADMIN_PASSWORD_RESET"
	MemberCreate              Type = "MEMBER_CREATE"
	MemberUpdate              Type = "MEMBER_UPDATE"
	MemberDelete              Type = "MEMBER_DELETE"
	MemberStatusChange        Type = "MEMBER_STATUS_CHANGE"
	ApplicationCreate         Type = "APPLICATION_CREATE"
	ApplicationUpdate         Type = "APPLICATION_UPDATE"
	ApplicationDelete         Type = "APPLICATION_DELETE"
	ApplicationStatusChange   Type = "APPLICATION_STATUS_CHANGE"
	PrincipalTokenQuotaAdd    Type = "PRINCIPAL_TOKEN_QUOTA_ADD"
	PrincipalTokenQuotaRemove Type = "PRINCIPAL_TOKEN_QUOTA_REMOVE"
	GroupApplicationAdd       Type = "GROUP_APPLICATION_ADD"
	GroupApplicationRemove    Type = "GROUP_APPLICATION_REMOVE"
	AccessKeyCreate           Type = "ACCESS_KEY_CREATE"
	AccessKeyRevoke           Type = "ACCESS_KEY_REVOKE"
	GroupCreate               Type = "GROUP_CREATE"
	GroupUpdate               Type = "GROUP_UPDATE"
	GroupStatusChange         Type = "GROUP_STATUS_CHANGE"
	GroupDelete               Type = "GROUP_DELETE"
	GroupTokenQuotaAdd        Type = "GROUP_TOKEN_QUOTA_ADD"
	GroupTokenQuotaRemove     Type = "GROUP_TOKEN_QUOTA_REMOVE"
	GroupMemberAdd            Type = "GROUP_MEMBER_ADD"
	GroupMemberRemove         Type = "GROUP_MEMBER_REMOVE"
	GroupModelGrant           Type = "GROUP_MODEL_GRANT"
	GroupModelRevoke          Type = "GROUP_MODEL_REVOKE"
	ModelStatusChange         Type = "MODEL_STATUS_CHANGE"
	ModelCreate               Type = "MODEL_CREATE"
	ModelUpdate               Type = "MODEL_UPDATE"
	ModelDelete               Type = "MODEL_DELETE"
	ModelCatalogSync          Type = "MODEL_CATALOG_SYNC"
	ProviderCreate            Type = "PROVIDER_CREATE"
	ProviderUpdate            Type = "PROVIDER_UPDATE"
	ProviderDelete            Type = "PROVIDER_DELETE"
	ProviderStatusChange      Type = "PROVIDER_STATUS_CHANGE"
	ResourceCreate            Type = "RESOURCE_CREATE"
	ResourceCredentialUpdate  Type = "RESOURCE_CREDENTIAL_UPDATE"
	ResourceCredentialExport  Type = "RESOURCE_CREDENTIAL_EXPORT"
	ResourceDelete            Type = "RESOURCE_DELETE"
	ResourceConnectionTest    Type = "RESOURCE_CONNECTION_TEST"
	ResourceRateLimitReset    Type = "RESOURCE_RATE_LIMIT_RESET"
	ResourcePriceUpdate       Type = "RESOURCE_PRICE_UPDATE"
	LoginSuccess              Type = "LOGIN_SUCCESS"
	LoginFailed               Type = "LOGIN_FAILED"
	LoginLocked               Type = "LOGIN_LOCKED"
)
