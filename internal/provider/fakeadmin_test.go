package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/terraprovider/go-exoscc/adminapi"
	"github.com/terraprovider/go-exoscc/exo"
	"github.com/terraprovider/terraform-provider-exo/internal/clients"
)

// fakeCall is one cmdlet invocation received by a fake Admin API.
type fakeCall struct {
	Cmdlet string
	Params map[string]any
}

// fakeAdminAPI serves InvokeCommand from respond and records every call.
type fakeAdminAPI struct {
	mu    sync.Mutex
	calls []fakeCall
}

func (f *fakeAdminAPI) Calls() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

// newFakeClient returns a client whose Admin API answers each cmdlet with respond's objects.
func newFakeClient(t *testing.T, respond func(cmdlet string, params map[string]any) []map[string]any) (*clients.Client, *fakeAdminAPI) {
	t.Helper()
	fake := &fakeAdminAPI{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CmdletInput struct {
				CmdletName string
				Parameters map[string]any
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in := body.CmdletInput
		fake.mu.Lock()
		fake.calls = append(fake.calls, fakeCall{in.CmdletName, in.Parameters})
		fake.mu.Unlock()
		value := respond(in.CmdletName, in.Parameters)
		if value == nil {
			value = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value})
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := adminapi.New(adminapi.Options{
		Cloud:      adminapi.Cloud{Name: "EXO", BaseHost: u.Host, Resource: "https://fake"},
		TenantID:   "00000000-0000-0000-0000-000000000000",
		Tokens:     adminapi.StaticTokenProvider("fake"),
		HTTPClient: srv.Client(),
		NoRetry:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &clients.Client{Admin: admin, EXO: exo.New(admin)}, fake
}
