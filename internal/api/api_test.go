package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewServer(service.New(st))
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status=%d want 200", rec.Code)
	}
}

func TestErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, domain.NewError(domain.CodeStaleCompatibility, false,
		domain.Reason{Message: "stale summary"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	var env ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Code != domain.CodeStaleCompatibility {
		t.Fatalf("code=%q want STALE_COMPATIBILITY", env.Code)
	}
	if len(env.Reasons) != 1 || env.Reasons[0] != "stale summary" {
		t.Fatalf("reasons=%v", env.Reasons)
	}
	if env.Retryable {
		t.Fatal("expected retryable=false")
	}
}

func TestLockEndpoint(t *testing.T) {
	s := newTestServer(t)
	body := `{"task_id":"T-1","building":"A","facade_zone":"E","panel":"P-017",
		"design_version":"dv-1","compatibility_ver":"cv-1","compat_valid_until":100000,
		"surface_summary":"s","batch":{"base_batch":"B","catalyst_batch":"C","primer_batch":"P"},
		"joints":[{"joint_id":"J-1","direction":"E","start":0,"end":3000,"width":20,"depth":10,
			"bond_area_um2":200,"segments":[{"seq":1,"start":0,"end":1000},{"seq":2,"start":1000,"end":2000},{"seq":3,"start":2000,"end":3000}],
			"trial_mapping":{"seg1":"sample-1"}}],
		"thresholds":{},"locked_at":100}`
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("lock status=%d body=%s", rec.Code, rec.Body.String())
	}
	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal lock: %v", err)
	}
	if res["generation"] != float64(1) {
		t.Fatalf("generation=%v want 1", res["generation"])
	}
}

func TestCommandEndpointAndQuery(t *testing.T) {
	s := newTestServer(t)
	lockBody := `{"task_id":"T-1","building":"A","facade_zone":"E","panel":"P-017",
		"design_version":"dv-1","compatibility_ver":"cv-1","compat_valid_until":100000,
		"surface_summary":"s","batch":{"base_batch":"B","catalyst_batch":"C","primer_batch":"P"},
		"joints":[{"joint_id":"J-1","direction":"E","start":0,"end":3000,"width":20,"depth":10,
			"bond_area_um2":200,"segments":[{"seq":1,"start":0,"end":1000},{"seq":2,"start":1000,"end":2000},{"seq":3,"start":2000,"end":3000}],
			"trial_mapping":{"seg1":"sample-1"}}],
		"thresholds":{},"locked_at":100}`
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(lockBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("lock status=%d body=%s", rec.Code, rec.Body.String())
	}

	cmdBody := `{"operation_id":"op-clean","kind":"clean","logical_time":101}`
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks/T-1/commands", strings.NewReader(cmdBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("command status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/tasks/T-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get task status=%d body=%s", rec.Code, rec.Body.String())
	}
	var view service.TaskView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal task view: %v", err)
	}
	if view.Stage != "CLEANED" {
		t.Fatalf("stage=%q want CLEANED", view.Stage)
	}
}
