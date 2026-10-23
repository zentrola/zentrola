package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/health"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

type memberManageStore struct {
	groups  []appsec.ManagedGroup
	after   int64
	limit   int32
	creates int
}

func (s *memberManageStore) CreateMemberAndKey(context.Context, appsec.PrincipalIdentity, int64, string, string, []int64, appsec.KeyRecord, appsec.RequestMeta) error {
	s.creates++
	return nil
}
func (*memberManageStore) IssueMemberKey(context.Context, appsec.PrincipalIdentity, appsec.KeyRecord, appsec.RequestMeta) error {
	return nil
}
func (*memberManageStore) SetMemberStatus(context.Context, appsec.PrincipalIdentity, int64, string, appsec.RequestMeta) (bool, error) {
	return false, nil
}
func (s *memberManageStore) ActiveGroups(_ context.Context, _ appsec.PrincipalIdentity, after int64, limit int32) ([]appsec.ManagedGroup, int64, error) {
	s.after, s.limit = after, limit
	return s.groups[:min(len(s.groups), int(limit))], int64(len(s.groups)), nil
}

type memberManageIDs struct{ next int64 }

func (s *memberManageIDs) NextID(context.Context) (int64, error) { s.next++; return s.next, nil }

func memberManageRouter(store *memberManageStore) http.Handler {
	identity := appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7, Type: "MEMBER", CanManage: true}
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: identity}, &memberManageIDs{next: 100})
	return NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod",
		&SecurityHandlers{Keys: keys, MemberProvisioning: appsec.NewMemberProvisioning(keys, store, nil)})
}

func memberManageRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer vk-"+base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestManagedGroupsPaginatesActiveGroups(t *testing.T) {
	store := &memberManageStore{groups: []appsec.ManagedGroup{{ID: 30, Name: "C", Status: "ACTIVE"}, {ID: 20, Name: "B", Status: "ACTIVE"}, {ID: 10, Name: "A", Status: "ACTIVE"}}}
	recorder := memberManageRequest(memberManageRouter(store), http.MethodGet, "/api/v1/me/manage/groups?limit=2&after=99", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data PageResponse[appsec.ManagedGroup] `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Items) != 2 || body.Data.NextCursor == nil || *body.Data.NextCursor != "20" || body.Data.Total != 3 {
		t.Fatalf("unexpected page: %+v", body.Data)
	}
	if store.after != 99 || store.limit != 3 {
		t.Fatalf("store received after=%d limit=%d, want 99 and probe limit 3", store.after, store.limit)
	}
	for _, query := range []string{"limit=0", "limit=101", "status=DISABLED"} {
		if recorder := memberManageRequest(memberManageRouter(store), http.MethodGet, "/api/v1/me/manage/groups?"+query, ""); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted: %d", query, recorder.Code)
		}
	}
}

func TestManagedMemberCreationRequiresGroup(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "missing groups", body: `{"name":"New","keyName":"First"}`, want: http.StatusBadRequest},
		{name: "empty groups", body: `{"name":"New","keyName":"First","groupIds":[]}`, want: http.StatusBadRequest},
		{name: "with group", body: `{"name":"New","keyName":"First","groupIds":["7"]}`, want: http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memberManageStore{}
			recorder := memberManageRequest(memberManageRouter(store), http.MethodPost, "/api/v1/me/manage/members", tc.body)
			wantCreates := 0
			if tc.want == http.StatusCreated {
				wantCreates = 1
			}
			if recorder.Code != tc.want || store.creates != wantCreates {
				t.Fatalf("status=%d creates=%d body=%s", recorder.Code, store.creates, recorder.Body.String())
			}
		})
	}
}
