package compiler_test

import (
	"github.com/jonace-mpelule/fQL/internal/generate"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"strings"
	"testing"
)

const policyModels = `
model Account {collection="accounts" fields {deviceId string.required() generation int.required() active bool.required()}}
model Envelope {collection="envelopes" fields {recipientId string.required().immutable() deviceId string.required().immutable() generation int.required().immutable() expiresAt timestamp.required() note string.optional()} access {%s}}
`

func TestTypedPolicies(t *testing.T) {
	source := `schema "0.1"
claims {deviceId string generation int}
policy activeDeviceFor(uid string, device string, generation int) {
 authenticated and auth.uid == uid and auth.claims.deviceId == device and auth.claims.generation == generation
 and get(Account,uid).deviceId == device and get(Account,uid).generation == generation and get(Account,uid).active == true
}
` + strings.Replace(policyModels, "%s", `get = activeDeviceFor(existing.recipientId,existing.deviceId,existing.generation) and existing.expiresAt > now
list = deny
create = authenticated and incoming.recipientId == auth.uid
update = owner(recipientId) and incoming.deviceId == existing.deviceId
`, 1)
	s, e := compile(source)
	if e != nil {
		t.Fatal(e)
	}
	rules, _, e := generate.Firestore(s)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"allow get:", "allow list: if false;", "request.auth.token[\"deviceId\"]", "get(/databases/$(database)/documents/accounts/$(resource.data[\"recipientId\"]))", "request.time", "valid(request.resource.data)"} {
		if !strings.Contains(string(rules), want) {
			t.Errorf("missing %s", want)
		}
	}
	// Revalidating a normalized graph is deterministic and retains semantic meaning.
	s, e = validate.Schema(s)
	if e != nil {
		t.Fatal(e)
	}
	again, _, e := generate.Firestore(s)
	if e != nil || string(again) != string(rules) {
		t.Fatalf("revalidation changed output: %v", e)
	}
}
func TestPolicyFailures(t *testing.T) {
	for name, expr := range map[string]string{
		"unknown field": "existing.unknown == auth.uid", "unknown claim": "auth.claims.unknown == existing.deviceId",
		"wrong type": "existing.generation == auth.uid", "unknown model": "get(Unknown,auth.uid).active == true",
		"unavailable state": "incoming.deviceId == auth.uid", "after read": "getAfter(Account,auth.uid).active == true",
		"optional": "existing.note == auth.uid", "nonboolean": "existing.deviceId", "invalid not": "not existing.deviceId",
		"unknown call": "execute(auth.uid)", "null": "existing.deviceId == null", "nonfinite": "existing.generation == 1e999",
		"unsafe path": `exists(Account,"a/b")`, "empty path": `exists(Account,"")`,
		"lookup id": "exists(Account,4)", "nested lookup": "get(Account,get(Account,auth.uid).deviceId).active == true",
	} {
		t.Run(name, func(t *testing.T) {
			_, e := compile(`schema "0.1"` + strings.Replace(policyModels, "%s", "get = "+expr, 1))
			if e == nil {
				t.Fatal("accepted invalid policy")
			}
			if !strings.Contains(e.Error(), "test.fQL:") {
				t.Fatalf("missing source location: %v", e)
			}
		})
	}
	for _, prefix := range []string{
		`claims {deviceId string deviceId int}`, `claims {deviceId timestamp}`, `claims {deviceId string} claims {generation int}`,
		`policy loop() {loop()}`, `policy first() {second()} policy second() {first()}`,
		`policy unused(x string) {x == 4}`, `policy implicit() {existing.deviceId == auth.uid}`,
		`policy unused(x string,x string) {true}`,
	} {
		if _, e := compile(`schema "0.1"` + prefix + strings.Replace(policyModels, "%s", "get = deny", 1)); e == nil {
			t.Fatalf("accepted %s", prefix)
		}
	}
	if _, e := compile(`schema "0.1"` + strings.Replace(policyModels, "%s", "read = authenticated get = deny", 1)); e == nil {
		t.Fatal("overlapping grant accepted")
	}
}
func TestPolicyBounds(t *testing.T) {
	for _, expr := range []string{strings.Repeat("not (", 40) + "true" + strings.Repeat(")", 40), strings.Repeat("true and ", 300) + "true"} {
		if _, e := compile(`schema "0.1"` + strings.Replace(policyModels, "%s", "get = "+expr, 1)); e == nil {
			t.Fatal("unbounded expression accepted")
		}
	}
	source := `schema "0.1"` + strings.Replace(policyModels, "%s", "get = "+strings.Repeat("get(Account,auth.uid).generation == 1 and ", 4)+"true", 1)
	s, e := compile(source)
	if e == nil {
		_, _, e = generate.Firestore(s)
	}
	if e == nil {
		t.Fatal("access call budget not enforced")
	}
}
func FuzzPolicyParser(f *testing.F) {
	f.Add("authenticated and not (auth.uid == existing.deviceId)")
	f.Add("getAfter(Account, auth.uid).generation >= 2")
	f.Fuzz(func(t *testing.T, expr string) {
		if len(expr) > 4000 {
			t.Skip()
		}
		s, e := syntax.Parse("fuzz.fql", []byte(`schema "0.1"`+strings.Replace(policyModels, "%s", "get = "+expr, 1)))
		if e == nil {
			if s, e = validate.Schema(s); e == nil {
				_, _, _ = generate.Firestore(s)
			}
		}
	})
}
