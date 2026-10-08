package http

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zentrola/zentrola/internal/domain/admin"
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
			// 部分集合必须显式传入，但允许用空数组表达清空配置。
			allowEmpty := fieldType.Tag.Get("allowempty") == "true" && field.Kind() == reflect.Slice && !field.IsNil()
			if hasRequiredBinding(fieldType.Tag.Get("binding")) && !allowEmpty && isMissingRequired(field) {
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
	// NewPassword 为 6～30 个字符，只允许可见的 ASCII 数字、英文字母和特殊符号，且不能与当前密码相同。
	NewPassword string `json:"newPassword" binding:"required"`
}

func (*ChangePasswordRequest) Normalize() {}
func (r ChangePasswordRequest) Valid() bool {
	return admin.ValidPasswordInput(r.CurrentPassword) && r.CurrentPassword != r.NewPassword && admin.ValidNewPassword(r.NewPassword)
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"your-admin-password"`
}

func (r *LoginRequest) Normalize() { r.Username = strings.TrimSpace(r.Username) }
func (r LoginRequest) Valid() bool {
	return validRequestText(r.Username, 64, true) && admin.ValidPasswordInput(r.Password)
}

type CreateKeyRequest struct {
	Name string `json:"name" binding:"required" example:"Claude Code"`
	// ExpiresAt 是以 Z 结尾的 UTC RFC3339 时间。
	ExpiresAt *time.Time `json:"expiresAt" example:"2027-01-01T00:00:00Z"`
}

func (r *CreateKeyRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }
func (r CreateKeyRequest) Valid() bool {
	return validRequestText(r.Name, 128, true) && isUTCDateTime(r.ExpiresAt) &&
		(r.ExpiresAt == nil || r.ExpiresAt.After(time.Now().UTC()))
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
	ProviderID  int64  `json:"providerId,string" binding:"required" swaggertype:"string" example:"123456789"`
	Name        string `json:"name" binding:"required" example:"企业模型资源"`
	Credential  string `json:"credential" binding:"required" example:"your-provider-api-key"`
	AuthType    string `json:"authType" enums:"API_KEY,SUBSCRIPTION" example:"API_KEY"`
	AuthAdapter string `json:"authAdapter" example:"API_KEY"`
	Priority    *int32 `json:"priority,omitempty" example:"100"`
	// EffectiveAt 是以 Z 结尾的 UTC RFC3339 时间。
	EffectiveAt *time.Time `json:"effectiveAt,omitempty" example:"2026-09-15T03:20:00Z"`
	// ExpiresAt 是以 Z 结尾的 UTC RFC3339 时间。
	ExpiresAt *time.Time `json:"expiresAt,omitempty" example:"2027-09-15T03:20:00Z"`
}

func (r *CreateResourceRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Credential = strings.TrimSpace(r.Credential)
	r.AuthType = strings.TrimSpace(r.AuthType)
	r.AuthAdapter = strings.TrimSpace(r.AuthAdapter)
	if r.AuthType == "" {
		r.AuthType = "API_KEY"
	}
	if r.AuthAdapter == "" && r.AuthType == "API_KEY" {
		r.AuthAdapter = "API_KEY"
	}
}
func (r CreateResourceRequest) Valid() bool {
	if r.ProviderID <= 0 || !validRequestText(r.Name, 128, true) || r.Priority != nil && *r.Priority < 0 ||
		!isUTCDateTime(r.EffectiveAt) || !isUTCDateTime(r.ExpiresAt) ||
		r.EffectiveAt != nil && r.ExpiresAt != nil && !r.ExpiresAt.After(*r.EffectiveAt) {
		return false
	}
	if r.AuthType == "API_KEY" {
		return r.AuthAdapter == "API_KEY" && validCredential(r.Credential)
	}
	return r.AuthType == "SUBSCRIPTION" &&
		(r.AuthAdapter == "OPENAI_CODEX" || r.AuthAdapter == "ANTHROPIC_CLAUDE_CODE") &&
		len(r.Credential) > 0 && len(r.Credential) <= 64<<10 && utf8.ValidString(r.Credential) &&
		!strings.ContainsRune(r.Credential, 0)
}

func isUTCDateTime(value *time.Time) bool {
	return value == nil || value.Location() == time.UTC
}

type UpdateCredentialRequest struct {
	Credential string `json:"credential" binding:"required" example:"your-provider-api-key"`
}

var credentialPriceDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$`)

type SaveModelPriceRequest struct {
	Currency         string `json:"currency" binding:"required" enums:"CNY,USD"`
	InputPrice       string `json:"inputPrice" binding:"required"`
	OutputPrice      string `json:"outputPrice" binding:"required"`
	CachedInputPrice string `json:"cachedInputPrice" binding:"required"`
	// EffectiveAt 是以 Z 结尾的 UTC 日期起点。
	EffectiveAt time.Time `json:"effectiveAt" binding:"required" example:"2026-10-01T00:00:00Z"`
}

func (*SaveModelPriceRequest) Normalize() {}
func (r SaveModelPriceRequest) Valid() bool {
	return (r.Currency == "CNY" || r.Currency == "USD") &&
		credentialPriceDecimal.MatchString(r.InputPrice) &&
		credentialPriceDecimal.MatchString(r.OutputPrice) &&
		credentialPriceDecimal.MatchString(r.CachedInputPrice) && validUTCDateStart(r.EffectiveAt)
}

func validUTCDateStart(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC &&
		value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

type SaveSubscriptionPriceRequest struct {
	Currency      string `json:"currency" binding:"required" enums:"CNY,USD"`
	PeriodAmount  string `json:"periodAmount" binding:"required"`
	BillingPeriod string `json:"billingPeriod" binding:"required" enums:"MONTH,YEAR"`
	// EffectiveAt 是以 Z 结尾的 UTC 日期起点。
	EffectiveAt time.Time `json:"effectiveAt" binding:"required" example:"2026-10-01T00:00:00Z"`
}

func (*SaveSubscriptionPriceRequest) Normalize() {}
func (r SaveSubscriptionPriceRequest) Valid() bool {
	return (r.Currency == "CNY" || r.Currency == "USD") &&
		credentialPriceDecimal.MatchString(r.PeriodAmount) &&
		(r.BillingPeriod == "MONTH" || r.BillingPeriod == "YEAR") &&
		validUTCDateStart(r.EffectiveAt)
}

func (r *UpdateCredentialRequest) Normalize() { r.Credential = strings.TrimSpace(r.Credential) }
func (r UpdateCredentialRequest) Valid() bool {
	return len(r.Credential) > 0 && len(r.Credential) <= 64<<10 && utf8.ValidString(r.Credential) && !strings.ContainsRune(r.Credential, 0)
}

type ConsumeResetCreditRequest struct {
	IdempotencyKey string `json:"idempotencyKey" binding:"required" example:"8ae96ff3-3425-4f4c-8772-b6fd61502868"`
	CreditID       string `json:"creditId,omitempty" example:"RateLimitResetCredit_1"`
}

func (r *ConsumeResetCreditRequest) Normalize() {
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	r.CreditID = strings.TrimSpace(r.CreditID)
}

func (r ConsumeResetCreditRequest) Valid() bool {
	return validRequestText(r.IdempotencyKey, 128, true) && validRequestText(r.CreditID, 256, false)
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required" enums:"ACTIVE,DISABLED" example:"ACTIVE"`
}

func (r *UpdateStatusRequest) Normalize() { r.Status = strings.TrimSpace(r.Status) }
func (r UpdateStatusRequest) Valid() bool { return r.Status == "ACTIVE" || r.Status == "DISABLED" }

type AddTokenQuotaRequest struct {
	Amount string `json:"amount" binding:"required" example:"1000000"`
	Reason string `json:"reason" binding:"required" example:"项目扩容"`
}

func (r *AddTokenQuotaRequest) Normalize() {
	r.Amount = strings.TrimSpace(r.Amount)
	r.Reason = strings.TrimSpace(r.Reason)
}

func (r AddTokenQuotaRequest) Valid() bool {
	amount, err := strconv.ParseInt(r.Amount, 10, 64)
	return err == nil && amount > 0 && validRequestText(r.Reason, 500, true)
}

func (r AddTokenQuotaRequest) TokenAmount() int64 {
	amount, _ := strconv.ParseInt(r.Amount, 10, 64)
	return amount
}
