package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func TestModel_CommandIdempotencyBoundary(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	server := NewServer(service.New(st))

	for _, taskID := range []string{"T-1", "T-2"} {
		body := fmt.Sprintf(`{"task_id":%q,"building":"A","facade_zone":"E","panel":%q,
			"design_version":"dv-1","compatibility_ver":"cv-1","compat_valid_until":100000,
			"surface_summary":"s","batch":{"base_batch":"B","catalyst_batch":"C","primer_batch":"P"},
			"joints":[{"joint_id":%q,"direction":"E","start":0,"end":3000,"width":20,"depth":10,
				"bond_area_um2":200,"segments":[{"seq":1,"start":0,"end":1000},{"seq":2,"start":1000,"end":2000},{"seq":3,"start":2000,"end":3000}],
				"trial_mapping":{"seg1":"sample-1"}}],"thresholds":{},"locked_at":100}`,
			taskID, "P-"+taskID, "J-"+taskID)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("lock %s: status=%d body=%s", taskID, rec.Code, rec.Body.String())
		}
	}

	var firstByTask = make(map[string]service.CommandResult)
	cases := []struct {
		name       string
		taskID     string
		body       string
		wantStatus int
		wantStage  string
		wantCode   domain.ErrorCode
		wantReplay bool
	}{
		{
			name:   "first task records the operation",
			taskID: "T-1", body: `{"operation_id":"shared-clean","kind":"clean","logical_time":101}`,
			wantStatus: http.StatusOK, wantStage: "CLEANED",
		},
		{
			name:   "same operation and content execute independently for another task",
			taskID: "T-2", body: `{"operation_id":"shared-clean","kind":"clean","logical_time":101}`,
			wantStatus: http.StatusOK, wantStage: "CLEANED",
		},
		{
			name:   "normalized retry on the same task replays its result",
			taskID: "T-2", body: `{"logical_time":101,"expected_generation":0,"kind":"clean","operation_id":"shared-clean"}`,
			wantStatus: http.StatusOK, wantStage: "CLEANED", wantReplay: true,
		},
		{
			name:   "different content on the same task conflicts",
			taskID: "T-2", body: `{"operation_id":"shared-clean","kind":"clean","logical_time":102}`,
			wantStatus: http.StatusConflict, wantCode: domain.CodeIdempotencyConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			path := "/v1/tasks/" + tc.taskID + "/commands"
			server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body)))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantCode != "" {
				var got ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if got.Code != tc.wantCode {
					t.Fatalf("code=%q want=%q", got.Code, tc.wantCode)
				}
				return
			}

			var got service.CommandResult
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode command response: %v", err)
			}
			if got.TaskID != tc.taskID || got.Stage != tc.wantStage {
				t.Fatalf("command result task=%q stage=%q want task=%q stage=%q", got.TaskID, got.Stage, tc.taskID, tc.wantStage)
			}
			if tc.wantReplay && got != firstByTask[tc.taskID] {
				t.Fatalf("retry result=%+v want original=%+v", got, firstByTask[tc.taskID])
			}
			if !tc.wantReplay {
				firstByTask[tc.taskID] = got
			}

			query := httptest.NewRecorder()
			server.ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+tc.taskID, nil))
			if query.Code != http.StatusOK {
				t.Fatalf("query status=%d body=%s", query.Code, query.Body.String())
			}
			var view service.TaskView
			if err := json.Unmarshal(query.Body.Bytes(), &view); err != nil {
				t.Fatalf("decode task view: %v", err)
			}
			if view.TaskID != tc.taskID || view.Stage != tc.wantStage {
				t.Fatalf("task view task=%q stage=%q want task=%q stage=%q", view.TaskID, view.Stage, tc.taskID, tc.wantStage)
			}
		})
	}
}
