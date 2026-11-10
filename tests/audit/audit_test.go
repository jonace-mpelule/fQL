package audit_test

import (
	"context"
	"encoding/json"
	"github.com/jonace-mpelule/fQL/internal/audit"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadOnlyPaginationValidationAndRedaction(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer owner" {
			t.Error("unexpected method/auth")
		}
		if r.URL.Query().Get("pageToken") == "" {
			w.Write([]byte(`{"documents":[{"name":"projects/demo-fql/databases/(default)/documents/users/a","fields":{"label":{"stringValue":"DO_NOT_REPORT_VALUE"}}}],"nextPageToken":"next"}`))
		} else {
			w.Write([]byte(`{"documents":[{"name":"projects/demo-fql/databases/(default)/documents/users/b","fields":{"label":{"integerValue":"1"}}}]}`))
		}
	}))
	defer server.Close()
	s := schema.Schema{Models: []schema.Model{{Name: "User", Collection: "users", Fields: []schema.Field{{Name: "label", Type: "string"}}}}}
	o := audit.Options{ProjectID: "demo-fql", Database: "(default)", Emulator: strings.TrimPrefix(server.URL, "http://"), MaxDocuments: 10}
	report, e := audit.Run(context.Background(), s, o)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(report)
	if !report.Complete || report.Scanned != 2 || len(report.Violations) != 1 || requests != 2 || strings.Contains(string(b), "DO_NOT_REPORT_VALUE") {
		t.Fatalf("unexpected audit %s", b)
	}
	o.MaxDocuments = 1
	report, e = audit.Run(context.Background(), s, o)
	if e != nil || report.Complete || report.Scanned != 1 {
		t.Fatalf("cap misreported: %+v %v", report, e)
	}
}
func TestTargetAndCredentialSafety(t *testing.T) {
	for _, o := range []audit.Options{{ProjectID: "demo-fql", Database: "(default)", MaxDocuments: 1}, {ProjectID: "demo-fql", Database: "(default)", MaxDocuments: 1, Emulator: "example.com:8080"}, {ProjectID: "../unsafe", Database: "(default)", MaxDocuments: 1, Emulator: "127.0.0.1:8080"}} {
		if _, e := audit.Run(context.Background(), schema.Schema{}, o); e == nil {
			t.Fatal("unsafe target accepted")
		}
	}
}
