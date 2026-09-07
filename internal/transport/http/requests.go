package http

import "time"

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	// NewPassword 为 12–72 个 UTF-8 字节，不能包含空字符，也不能与当前密码相同。
	NewPassword string `json:"newPassword" binding:"required"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"your-admin-password"`
}

type CreateKeyRequest struct {
	Name      string     `json:"name" binding:"required" example:"Claude Code"`
	ExpiresAt *time.Time `json:"expiresAt" example:"2027-01-01T00:00:00Z"`
}

type CreateMemberRequest struct {
	Name     string   `json:"name" binding:"required" example:"开发者"`
	Remark   string   `json:"remark" example:"研发成员"`
	GroupIDs []string `json:"groupIds" example:"123456789,987654321"`
}

type UpdateMemberRequest struct {
	Name     string   `json:"name" binding:"required" example:"开发者"`
	Remark   string   `json:"remark" example:"研发成员"`
	GroupIDs []string `json:"groupIds" example:"123456789,987654321"`
}

type CreateGroupRequest struct {
	// Code 仅为兼容旧客户端保留；为空时由服务端生成内部编码。
	Code     string   `json:"code,omitempty" example:"engineering"`
	Name     string   `json:"name" binding:"required" example:"研发组"`
	Remark   string   `json:"remark" example:"研发模型权限组"`
	ModelIDs []string `json:"modelIds" example:"123456789,987654321"`
}
type UpdateGroupRequest struct {
	Name     string   `json:"name" binding:"required" example:"研发组"`
	Remark   string   `json:"remark" example:"研发模型权限组"`
	ModelIDs []string `json:"modelIds" example:"123456789,987654321"`
}

type CreateResourceRequest struct {
	ProviderID int64  `json:"providerId,string" binding:"required" swaggertype:"string" example:"123456789"`
	Name       string `json:"name" binding:"required" example:"企业模型资源"`
	Credential string `json:"credential" binding:"required" example:"your-provider-api-key"`
}

type UpdateCredentialRequest struct {
	Credential string `json:"credential" binding:"required" example:"your-provider-api-key"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required" enums:"ACTIVE,DISABLED" example:"ACTIVE"`
}
