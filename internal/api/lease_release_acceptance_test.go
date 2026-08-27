package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestModel_LeaseReleaseResponseReflectsPersistentOutcome(t *testing.T) {
	type response struct {
		Status string           `json:"status"`
		Code   domain.ErrorCode `json:"code"`
	}
	tests := []struct {
		name              string
		acquireOriginal   bool
		originalExpiresAt int64
		releaseToken      string
		releaseAt         int64
		wantStatus        int
		wantCode          domain.ErrorCode
		wantReacquire     int
	}{
		{
			name:            "wrong token reports conflict and preserves holder",
			acquireOriginal: true, originalExpiresAt: 100,
			releaseToken: "wrong-token", releaseAt: 10,
			wantStatus: http.StatusUnprocessableEntity, wantCode: domain.CodeLeaseConflict,
			wantReacquire: http.StatusUnprocessableEntity,
		},
		{
			name:         "missing lease reports expired instead of released",
			releaseToken: "original-token", releaseAt: 10,
			wantStatus: http.StatusUnprocessableEntity, wantCode: domain.CodeLeaseExpired,
			wantReacquire: http.StatusOK,
		},
		{
			name:            "expired lease reports expired instead of released",
			acquireOriginal: true, originalExpiresAt: 20,
			releaseToken: "original-token", releaseAt: 20,
			wantStatus: http.StatusUnprocessableEntity, wantCode: domain.CodeLeaseExpired,
			wantReacquire: http.StatusOK,
		},
		{
			name:            "matching token is committed before released response",
			acquireOriginal: true, originalExpiresAt: 100,
			releaseToken: "original-token", releaseAt: 10,
			wantStatus:    http.StatusOK,
			wantReacquire: http.StatusOK,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			resourceID := tc.name
			request := func(path, body string) (int, response) {
				t.Helper()
				rec := httptest.NewRecorder()
				s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
				var got response
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode %s response (status %d, body %q): %v", path, rec.Code, rec.Body.String(), err)
				}
				return rec.Code, got
			}
			acquireBody := func(token, holder string, acquiredAt, expiresAt int64) string {
				b, err := json.Marshal(map[string]interface{}{
					"resource_type": domain.ResourceMixer,
					"resource_id":   resourceID,
					"token":         token,
					"holder_op":     holder,
					"acquired_at":   acquiredAt,
					"expires_at":    expiresAt,
				})
				if err != nil {
					t.Fatalf("marshal acquire request: %v", err)
				}
				return string(b)
			}

			if tc.acquireOriginal {
				status, _ := request("/v1/leases/acquire", acquireBody("original-token", "original-holder", 1, tc.originalExpiresAt))
				if status != http.StatusOK {
					t.Fatalf("initial acquire status=%d want %d", status, http.StatusOK)
				}
			}

			releaseJSON, err := json.Marshal(map[string]interface{}{
				"resource_type": domain.ResourceMixer,
				"resource_id":   resourceID,
				"token":         tc.releaseToken,
				"at":            tc.releaseAt,
			})
			if err != nil {
				t.Fatalf("marshal release request: %v", err)
			}
			status, got := request("/v1/leases/release", string(releaseJSON))
			if status != tc.wantStatus || got.Code != tc.wantCode {
				t.Fatalf("release status=%d code=%q status-text=%q; want status=%d code=%q", status, got.Code, got.Status, tc.wantStatus, tc.wantCode)
			}
			if tc.wantStatus == http.StatusOK && got.Status != "released" {
				t.Fatalf("release response status-text=%q want released", got.Status)
			}

			reacquireAt := tc.releaseAt
			if reacquireAt < tc.originalExpiresAt && tc.wantReacquire == http.StatusOK {
				reacquireAt = tc.releaseAt
			}
			reacquireStatus, reacquire := request("/v1/leases/acquire", acquireBody("replacement-token", "replacement-holder", reacquireAt, 200+int64(i)))
			if reacquireStatus != tc.wantReacquire {
				t.Fatalf("immediate reacquire status=%d code=%q want status=%d", reacquireStatus, reacquire.Code, tc.wantReacquire)
			}
		})
	}
}
