package http

import "time"

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"your-admin-password"`
}

type CreateKeyRequest struct {
	Name      string     `json:"name" binding:"required" example:"Claude Code"`
	ExpiresAt *time.Time `json:"expiresAt" example:"2027-01-01T00:00:00Z"`
}

type CreateMemberRequest struct {
	Name   string `json:"name" binding:"required" example:"开发者"`
	Remark string `json:"remark" example:"研发成员"`
}

type CreateGroupRequest struct {
	Code   string `json:"code" binding:"required" example:"engineering"`
	Name   string `json:"name" binding:"required" example:"研发组"`
	Remark string `json:"remark" example:"研发模型权限组"`
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
