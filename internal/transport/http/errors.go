package http

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// securityError 是管理 API 的统一应用错误出口。错误语义属于 application，
// HTTP 状态码、稳定响应 code 与面向客户端的消息只在 transport 层维护。
func securityError(w http.ResponseWriter, r *http.Request, err error) {
	var gatewayFailure *gw.Failure
	if errors.As(err, &gatewayFailure) {
		writeJSON(w, r, gatewayFailure.Status, response{Code: gatewayFailure.Code, Message: gatewayFailure.Message})
		return
	}
	var missing *missingRequiredParameterError
	if errors.As(err, &missing) {
		writeJSON(w, r, http.StatusBadRequest, response{
			Code:    "MISSING_REQUIRED_PARAMETER",
			Message: missing.Error(),
			Data:    map[string]string{"field": missing.Field},
		})
		return
	}
	var locked *appsec.AccountLockedError
	if errors.As(err, &locked) {
		remaining := time.Until(locked.LockedUntil)
		seconds := max(1, int64((remaining+time.Second-1)/time.Second))
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
		writeJSON(w, r, http.StatusTooManyRequests, response{
			Code: "ACCOUNT_LOCKED", Message: "Account temporarily locked.",
			Data: LoginLockResponse{LockedUntil: locked.LockedUntil.UTC(), RetryAfterSeconds: seconds},
		})
		return
	}
	status, code, message := http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service unavailable."
	switch {
	case errors.Is(err, appsec.ErrCurrentPassword):
		status, code, message = http.StatusForbidden, "CURRENT_PASSWORD_INCORRECT", "Current password is incorrect."
	case errors.Is(err, appsec.ErrAlreadyInitialized):
		status, code, message = http.StatusConflict, "ALREADY_INITIALIZED", "Administrator setup is already complete."
	case errors.Is(err, appsec.ErrUnauthenticated):
		status, code, message = http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication failed."
	case errors.Is(err, appsec.ErrInvalidArgument):
		status, code, message = http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request parameter."
	case errors.Is(err, appsec.ErrNotFound):
		status, code, message = http.StatusNotFound, "NOT_FOUND", "Object not found."
	case errors.Is(err, mgmt.ErrConflict):
		status, code, message = http.StatusConflict, "CONFLICT", "The request conflicts with the current state."
	case errors.Is(err, mgmt.ErrMemberAccessKeyRequired):
		status, code, message = http.StatusConflict, "MEMBER_ACCESS_KEY_REQUIRED", "Create an access key before enabling the member."
	case errors.Is(err, mgmt.ErrSubscriptionAccountExists):
		status, code, message = http.StatusConflict, "SUBSCRIPTION_ACCOUNT_ALREADY_EXISTS", "A credential for this subscription account already exists."
	case errors.Is(err, mgmt.ErrCredential):
		status, code, message = http.StatusUnprocessableEntity, "CREDENTIAL_UNRECOVERABLE", "Replace the resource credential before enabling it."
	case errors.Is(err, mgmt.ErrProvider):
		status, code, message = http.StatusConflict, "PROVIDER_UNAVAILABLE", "Provider is unavailable."
	case errors.Is(err, mgmt.ErrProviderCredentialRequired):
		status, code, message = http.StatusConflict, "PROVIDER_CREDENTIAL_REQUIRED", "Configure a provider credential before enabling the provider."
	case errors.Is(err, mgmt.ErrProviderModelMappingRequired):
		status, code, message = http.StatusConflict, "PROVIDER_MODEL_MAPPING_REQUIRED", "Configure a mapping to an active model before enabling the provider."
	case errors.Is(err, mgmt.ErrModelSyncCredentialRequired):
		status, code, message = http.StatusConflict, "MODEL_SYNC_CREDENTIAL_REQUIRED", "Configure a provider credential before synchronizing models."
	case errors.Is(err, mgmt.ErrCredentialExportUnsupported):
		status, code, message = http.StatusConflict, "CREDENTIAL_EXPORT_UNSUPPORTED", "Only OpenAI personal subscription credentials can be exported."
	case errors.Is(err, mgmt.ErrResetCreditUnsupported):
		status, code, message = http.StatusConflict, "RESET_CREDIT_UNSUPPORTED", "Only OpenAI personal subscriptions support rate-limit reset credits."
	}
	writeJSON(w, r, status, response{Code: code, Message: message})
}
