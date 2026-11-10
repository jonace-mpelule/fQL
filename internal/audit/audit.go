// Package audit performs explicit, bounded, read-only Firestore REST scans.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/data"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	ProjectID, Database, Model, Emulator, Token string
	MaxDocuments                                int
	Client                                      *http.Client
}
type Violation struct {
	Model    string       `json:"model"`
	Document string       `json:"document"`
	Issues   []data.Issue `json:"issues"`
}
type Report struct {
	Environment string      `json:"environment"`
	Scanned     int         `json:"scanned"`
	Complete    bool        `json:"complete"`
	Violations  []Violation `json:"violations"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._()-]{0,99}$`)

func Run(ctx context.Context, s schema.Schema, o Options) (Report, error) {
	r := Report{Environment: "production", Complete: true, Violations: []Violation{}}
	if !identifier.MatchString(o.ProjectID) || (o.Database != "(default)" && !identifier.MatchString(o.Database)) || o.MaxDocuments < 1 || o.MaxDocuments > 1000000 {
		return r, fmt.Errorf("audit requires a valid --firebase-project, --database, and --max-documents from 1 to 1000000")
	}
	base := "https://firestore.googleapis.com/v1"
	token := o.Token
	if o.Emulator != "" {
		host, port, e := net.SplitHostPort(o.Emulator)
		if e != nil {
			return r, fmt.Errorf("--emulator must be a loopback host:port")
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return r, fmt.Errorf("emulator auditing is restricted to loopback hosts")
		}
		if n, e := strconv.Atoi(port); e != nil || n < 1 || n > 65535 {
			return r, fmt.Errorf("invalid emulator port")
		}
		base = "http://" + net.JoinHostPort(host, port) + "/v1"
		token = "owner"
		r.Environment = "emulator"
	} else if token == "" {
		return r, fmt.Errorf("production audit requires an OAuth access token in FQL_ACCESS_TOKEN (read-only IAM access is recommended)")
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("audit redirects are disabled") }
	client = &copyClient
	var models []schema.Model
	for _, m := range s.Models {
		if o.Model == "" || o.Model == m.Name {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return r, fmt.Errorf("unknown --model")
	}
	for mi, m := range models {
		page := ""
		emptyPages := 0
		seen := map[string]bool{}
		for {
			remaining := o.MaxDocuments - r.Scanned
			if remaining <= 0 {
				r.Complete = false
				return r, nil
			}
			size := 100
			if remaining < size {
				size = remaining
			}
			endpoint := base + "/projects/" + url.PathEscape(o.ProjectID) + "/databases/" + url.PathEscape(o.Database) + "/documents/" + url.PathEscape(m.Collection)
			query := url.Values{"pageSize": {strconv.Itoa(size)}}
			if page != "" {
				query.Set("pageToken", page)
			}
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
			if e != nil {
				return r, fmt.Errorf("could not construct audit request")
			}
			req.Header.Set("Authorization", "Bearer "+token)
			response, e := client.Do(req)
			if e != nil {
				return r, fmt.Errorf("audit request failed; check connectivity and credentials")
			}
			b, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
			response.Body.Close()
			if readErr != nil || len(b) > 16<<20 {
				return r, fmt.Errorf("audit response exceeded its size budget")
			}
			if response.StatusCode != 200 {
				return r, fmt.Errorf("audit request returned HTTP %d; no response contents or credentials are logged", response.StatusCode)
			}
			var result struct {
				Documents []struct {
					Name   string                     `json:"name"`
					Fields map[string]json.RawMessage `json:"fields"`
				} `json:"documents"`
				Next string `json:"nextPageToken"`
			}
			if e = json.Unmarshal(b, &result); e != nil {
				return r, fmt.Errorf("invalid audit response")
			}
			if len(result.Documents) == 0 && result.Next != "" {
				emptyPages++
				if emptyPages > 3 {
					return r, fmt.Errorf("audit pagination exceeded the empty-page budget")
				}
			}
			if len(result.Documents) > size {
				return r, fmt.Errorf("audit response exceeded requested page size")
			}
			for _, doc := range result.Documents {
				prefix := "projects/" + o.ProjectID + "/databases/" + o.Database + "/documents/" + m.Collection + "/"
				if !strings.HasPrefix(doc.Name, prefix) || strings.Contains(strings.TrimPrefix(doc.Name, prefix), "/") {
					return r, fmt.Errorf("audit received an unexpected document path")
				}
				values := map[string]any{}
				for name, v := range doc.Fields {
					values[name] = decode(v, 0)
				}
				issues := data.Validate(m.Fields, values)
				sort.Slice(issues, func(i, j int) bool { return issues[i].Field < issues[j].Field })
				if len(issues) > 0 {
					r.Violations = append(r.Violations, Violation{m.Name, doc.Name, issues})
				}
				r.Scanned++
			}
			if result.Next == "" {
				if r.Scanned >= o.MaxDocuments && mi < len(models)-1 {
					r.Complete = false
				}
				break
			}
			if seen[result.Next] {
				return r, fmt.Errorf("audit server repeated a page token")
			}
			seen[result.Next] = true
			page = result.Next
		}
		if !r.Complete {
			return r, nil
		}
	}
	return r, nil
}
func decode(raw json.RawMessage, depth int) any {
	if depth > 8 {
		return data.Invalid{}
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || len(value) != 1 {
		return data.Invalid{}
	}
	for kind, b := range value {
		switch kind {
		case "nullValue":
			var x any
			if json.Unmarshal(b, &x) != nil || x != nil && x != "NULL_VALUE" {
				return data.Invalid{}
			}
			return nil
		case "stringValue":
			var x string
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			return x
		case "booleanValue":
			var x bool
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			return x
		case "integerValue":
			var x string
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			n, e := strconv.ParseInt(x, 10, 64)
			if e != nil {
				return data.Invalid{}
			}
			return n
		case "doubleValue":
			var x float64
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			return x
		case "timestampValue":
			var x string
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			return data.Timestamp(x)
		case "mapValue":
			var x struct {
				Fields map[string]json.RawMessage `json:"fields"`
			}
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			out := map[string]any{}
			for k, v := range x.Fields {
				out[k] = decode(v, depth+1)
			}
			return out
		case "arrayValue":
			var x struct {
				Values []json.RawMessage `json:"values"`
			}
			if json.Unmarshal(b, &x) != nil {
				return data.Invalid{}
			}
			out := []any{}
			for _, v := range x.Values {
				out = append(out, decode(v, depth+1))
			}
			return out
		}
	}
	return data.Invalid{}
}
