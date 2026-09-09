package http

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type missingRequiredParameterError struct {
	Field string
}

func (e *missingRequiredParameterError) Error() string {
	return fmt.Sprintf("Required parameter %q is missing.", e.Field)
}

func hasRequiredBinding(tag string) bool {
	for _, option := range strings.Split(tag, ",") {
		if option == "required" {
			return true
		}
	}
	return false
}

func jsonFieldName(field reflect.StructField) string {
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "-" {
		return ""
	}
	if name == "" {
		return field.Name
	}
	return name
}

func fieldPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func isMissingRequired(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.String:
		return strings.TrimSpace(value.String()) == ""
	case reflect.Array, reflect.Slice, reflect.Map:
		return value.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return value.IsNil()
	default:
		return value.IsZero()
	}
}

func missingRequiredParameter(value reflect.Value, prefix string) string {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		typeOfValue := value.Type()
		for index := 0; index < value.NumField(); index++ {
			fieldType := typeOfValue.Field(index)
			if fieldType.PkgPath != "" {
				continue
			}
			name := jsonFieldName(fieldType)
			if name == "" {
				continue
			}
			field := value.Field(index)
			path := fieldPath(prefix, name)
			if hasRequiredBinding(fieldType.Tag.Get("binding")) && isMissingRequired(field) {
				return path
			}
			if !field.IsZero() {
				if missing := missingRequiredParameter(field, path); missing != "" {
					return missing
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			path := fmt.Sprintf("%s[%d]", prefix, index)
			if missing := missingRequiredParameter(value.Index(index), path); missing != "" {
				return missing
			}
		}
	}
	return ""
}

func normalizeStrings(values []string) {
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
}

func validRequestText(value string, maxBytes int, required bool) bool {
	return (!required || value != "") && len(value) <= maxBytes && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validPassword(value string, minCharacters int) bool {
	return utf8.RuneCountInString(value) >= minCharacters && len(value) <= 72 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validRequestIDs(values []string) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		id, err := positiveID(value)
		if err != nil {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validCredential(value string) bool {
	if len(value) == 0 || len(value) > 4096 {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	// NewPassword 至少 6 个字符且不超过 72 个 UTF-8 字节，不能包含空字符，也不能与当前密码相同。
	NewPassword string `json:"newPassword" binding:"required"`
}

func (*ChangePasswordRequest) Normalize() {}
func (r ChangePasswordRequest) Valid() bool {
	return validPassword(r.CurrentPassword, 1) && r.CurrentPassword != r.NewPassword && validPassword(r.NewPassword, 6)
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"your-admin-password"`
}

func (r *LoginRequest) Normalize() { r.Username = strings.TrimSpace(r.Username) }
func (r LoginRequest) Valid() bool {
	return validRequestText(r.Username, 64, true) && validPassword(r.Password, 1)
}

type CreateKeyRequest struct {
	Name      string     `json:"name" binding:"required" example:"Claude Code"`
	ExpiresAt *time.Time `json:"expiresAt" example:"2027-01-01T00:00:00Z"`
}

func (r *CreateKeyRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }
func (r CreateKeyRequest) Valid() bool {
	return validRequestText(r.Name, 128, true) && (r.ExpiresAt == nil || r.ExpiresAt.After(time.Now().UTC()))
}

type CreateMemberRequest struct {
	Name     string   `json:"name" binding:"required" example:"开发者"`
	Remark   string   `json:"remark" example:"研发成员"`
	GroupIDs []string `json:"groupIds" example:"123456789,987654321"`
}

func (r *CreateMemberRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Remark = strings.TrimSpace(r.Remark)
	normalizeStrings(r.GroupIDs)
}
func (r CreateMemberRequest) Valid() bool {
	return validRequestText(r.Name, 128, true) && validRequestText(r.Remark, 2000, false) && validRequestIDs(r.GroupIDs)
}

type UpdateMemberRequest struct {
	Name     string   `json:"name" binding:"required" example:"开发者"`
	Remark   string   `json:"remark" example:"研发成员"`
	GroupIDs []string `json:"groupIds" example:"123456789,987654321"`
}

func (r *UpdateMemberRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Remark = strings.TrimSpace(r.Remark)
	normalizeStrings(r.GroupIDs)
}
func (r UpdateMemberRequest) Valid() bool {
	return validRequestText(r.Name, 128, true) && validRequestText(r.Remark, 2000, false) && validRequestIDs(r.GroupIDs)
}

type CreateGroupRequest struct {
	// Code 仅为兼容旧客户端保留；为空时由服务端生成内部编码。
	Code     string   `json:"code,omitempty" example:"engineering"`
	Name     string   `json:"name" binding:"required" example:"研发组"`
	Remark   string   `json:"remark" example:"研发模型权限组"`
	ModelIDs []string `json:"modelIds" example:"123456789,987654321"`
}

func (r *CreateGroupRequest) Normalize() {
	r.Code = strings.TrimSpace(r.Code)
	r.Name = strings.TrimSpace(r.Name)
	r.Remark = strings.TrimSpace(r.Remark)
	normalizeStrings(r.ModelIDs)
}
func (r CreateGroupRequest) Valid() bool {
	return validRequestText(r.Code, 64, false) && validRequestText(r.Name, 128, true) &&
		validRequestText(r.Remark, 2000, false) && validRequestIDs(r.ModelIDs)
}

type UpdateGroupRequest struct {
	Name     string   `json:"name" binding:"required" example:"研发组"`
	Remark   string   `json:"remark" example:"研发模型权限组"`
	ModelIDs []string `json:"modelIds" example:"123456789,987654321"`
}

func (r *UpdateGroupRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Remark = strings.TrimSpace(r.Remark)
	normalizeStrings(r.ModelIDs)
}
func (r UpdateGroupRequest) Valid() bool {
	return validRequestText(r.Name, 128, true) && validRequestText(r.Remark, 2000, false) && validRequestIDs(r.ModelIDs)
}

type CreateResourceRequest struct {
	ProviderID int64  `json:"providerId,string" binding:"required" swaggertype:"string" example:"123456789"`
	Name       string `json:"name" binding:"required" example:"企业模型资源"`
	Credential string `json:"credential" binding:"required" example:"your-provider-api-key"`
}

func (r *CreateResourceRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Credential = strings.TrimSpace(r.Credential)
}
func (r CreateResourceRequest) Valid() bool {
	return r.ProviderID > 0 && validRequestText(r.Name, 128, true) && validCredential(r.Credential)
}

type UpdateCredentialRequest struct {
	Credential string `json:"credential" binding:"required" example:"your-provider-api-key"`
}

func (r *UpdateCredentialRequest) Normalize() { r.Credential = strings.TrimSpace(r.Credential) }
func (r UpdateCredentialRequest) Valid() bool { return validCredential(r.Credential) }

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required" enums:"ACTIVE,DISABLED" example:"ACTIVE"`
}

func (r *UpdateStatusRequest) Normalize() { r.Status = strings.TrimSpace(r.Status) }
func (r UpdateStatusRequest) Valid() bool { return r.Status == "ACTIVE" || r.Status == "DISABLED" }
