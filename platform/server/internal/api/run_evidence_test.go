package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"testing"
)

func TestRunScreenshotEvidenceRoundTrip(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	server := New(st)
	handler := server.Routes()
	var screenshot bytes.Buffer
	if err := jpeg.Encode(&screenshot, image.NewGray(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	finish := []byte(mustJSON(map[string]any{"status": "failed", "steps": []map[string]any{
		{"step": 1, "action": "navigate", "status": "passed"},
		{"step": 2, "action": "fill", "status": "passed", "screenshot_error": "unavailable"},
		{"step": 3, "action": "assert_text", "status": "failed", "screenshot_jpeg": screenshot.Bytes()},
	}}))
	const detailPath = "/api/runs/run-a"
	if response := serveAgent(server, detailPath+"/finish", "tenant-b", finish); response.Code != http.StatusNotFound {
		t.Fatalf("foreign finish: %d", response.Code)
	}
	if response := serveAgent(server, detailPath+"/finish", "tenant-a", finish); response.Code != http.StatusOK {
		t.Fatalf("finish: %d %s", response.Code, response.Body)
	}
	response := serveTenant(handler, http.MethodGet, detailPath, "tenant-a", nil)
	var detail struct {
		Steps []struct {
			ScreenshotJPEG  []byte `json:"screenshot_jpeg"`
			ScreenshotError string `json:"screenshot_error"`
		} `json:"steps"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &detail) != nil || len(detail.Steps) != 3 {
		t.Fatalf("detail: %d %s", response.Code, response.Body)
	}
	if len(detail.Steps[0].ScreenshotJPEG) != 0 || detail.Steps[1].ScreenshotError != "unavailable" || !bytes.Equal(detail.Steps[2].ScreenshotJPEG, screenshot.Bytes()) {
		t.Fatal("screenshot evidence did not round-trip on the expected step")
	}
	for _, lookup := range []struct{ path, tenant string }{{detailPath, "tenant-b"}, {"/api/runs", "tenant-a"}} {
		response := serveTenant(handler, http.MethodGet, lookup.path, lookup.tenant, nil)
		if bytes.Contains(response.Body.Bytes(), []byte("screenshot_jpeg")) {
			t.Fatalf("screenshot included in %s for %s", lookup.path, lookup.tenant)
		}
	}
}

func TestRunFinishRejectsInvalidScreenshotEvidence(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	server := New(st)
	var screenshot bytes.Buffer
	if err := jpeg.Encode(&screenshot, image.NewGray(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		image  any
		count  int
		reason string
	}{
		{name: "malformed base64", image: "!", count: 1},
		{name: "non JPEG", image: []byte("not an image"), count: 1},
		{name: "oversized", image: append(bytes.Clone(screenshot.Bytes()), make([]byte, 128*1024)...), count: 1},
		{name: "too many", image: screenshot.Bytes(), count: 5},
		{name: "conflicting fields", image: screenshot.Bytes(), count: 1, reason: "unavailable"},
		{name: "unknown reason", count: 1, reason: "raw browser exception"},
	} {
		t.Run(test.name, func(t *testing.T) {
			steps := make([]map[string]any, test.count)
			for index := range steps {
				steps[index] = map[string]any{"step": index + 1, "action": "click", "status": "passed", "screenshot_jpeg": test.image, "screenshot_error": test.reason}
			}
			finish := []byte(mustJSON(map[string]any{"status": "passed", "steps": steps}))
			response := serveAgent(server, "/api/runs/run-a/finish", "tenant-a", finish)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("finish: %d %s", response.Code, response.Body)
			}
		})
	}
}
