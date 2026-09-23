package mailvendor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mustList(t *testing.T, c Client) []Mailbox {
	t.Helper()
	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return got
}

func mustCreds(t *testing.T, c Client, m Mailbox) Credentials {
	t.Helper()
	cr, err := c.Credentials(context.Background(), m)
	if err != nil {
		t.Fatalf("Credentials(%s): %v", m.Email, err)
	}
	return cr
}

func assertHeader(t *testing.T, r seenRequest, name, want string) {
	t.Helper()
	if got := r.Header.Get(name); got != want {
		t.Errorf("%s %s: header %s = %q, want %q", r.Method, r.Path, name, got, want)
	}
}

// InboxKit list items, shaped like the documented example.
func inboxKitMailbox(uid, user, domain, platform string) string {
	return fmt.Sprintf(`{"uid":%q,"domain_name":%q,"forwarding_email":"","first_name":"marvin","last_name":"kassulke","profile_picture":null,"username":%q,"platform":%q,"status":"active","is_admin":false,"tags":[],"sequencers":[],"createdAt":"2025-06-15T11:53:54.260Z","updatedAt":"2025-06-15T12:12:57.506Z","renewal_date":"1970-01-01T00:00:00.000Z","mailbox_update_status":"queued","sequencer_status":"queued","dns_propagation_status":"queued","renewal_cycle":"monthly","renewal_status":"na","mailbox_reactivation_status":"na"}`, uid, domain, user, platform)
}

func TestInboxKit(t *testing.T) {
	const ws = "6f1c2d3e-0000-4000-8000-000000000001"
	srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/api/mailboxes/list":
			var body struct{ Page, Limit int }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Page == 1 {
				writeJSON(w, 200, `{"error":false,"message":"Mailboxes retrieved successfully","mailboxes":[`+
					inboxKitMailbox("e88ae415-fe99-4831-b3fe-cdf2e7e25925", "marvinkassulke", "myzyng.net", "GOOGLE")+
					`],"total":2,"pages":2,"current_page":1,"limit":1}`)
				return
			}
			writeJSON(w, 200, `{"error":false,"message":"Mailboxes retrieved successfully","mailboxes":[`+
				inboxKitMailbox("0b7c7a8a-1111-4222-8333-444455556666", "jane", "acme.io", "MICROSOFT")+
				`],"total":2,"pages":2,"current_page":2,"limit":1}`)
		case "/v1/api/mailboxes/show-credentials":
			if r.URL.Query().Get("uid") == "gone" {
				writeJSON(w, 404, `{"error":true,"message":"Mailbox not found"}`)
				return
			}
			writeJSON(w, 200, `{"error":false,"message":"Mailbox credentials retrieved successfully","password":"pw-12345","secret":"TOTPSECRET","app_password":"abcd efgh ijkl mnop"}`)
		default:
			writeJSON(w, 404, `{}`)
		}
	})
	c := newTestClient(t, VendorInboxKit, map[string]string{FieldAPIKey: testKey, FieldWorkspaceID: ws}, srv.URL, nil)

	got := mustList(t, c)
	want := []Mailbox{
		{ID: "e88ae415-fe99-4831-b3fe-cdf2e7e25925", Email: "marvinkassulke@myzyng.net", FirstName: "marvin", LastName: "kassulke", Domain: "myzyng.net", Provider: ProviderGoogle, Status: "active"},
		{ID: "0b7c7a8a-1111-4222-8333-444455556666", Email: "jane@acme.io", FirstName: "marvin", LastName: "kassulke", Domain: "acme.io", Provider: ProviderMicrosoft, Status: "active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v\nwant %+v", got, want)
	}
	cr := mustCreds(t, c, got[0])
	if cr != (Credentials{Password: "pw-12345", AppPassword: "abcd efgh ijkl mnop"}) {
		t.Fatalf("Credentials = %+v", cr)
	}
	if _, err := c.Credentials(context.Background(), Mailbox{ID: "gone"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("gone: %v", err)
	}

	reqs := srv.requests()
	for _, r := range reqs {
		assertHeader(t, r, "Authorization", "Bearer "+testKey)
		assertHeader(t, r, "X-Workspace-Id", ws)
	}
	if reqs[0].Method != http.MethodPost || !strings.Contains(reqs[0].Body, `"page":1`) || !strings.Contains(reqs[1].Body, `"page":2`) {
		t.Fatalf("list requests = %+v", reqs[:2])
	}
	if reqs[2].Query["uid"][0] != want[0].ID {
		t.Fatalf("show-credentials query = %v", reqs[2].Query)
	}
}

func TestInboxKitErrorEnvelope(t *testing.T) {
	srv := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"error":true,"message":"Workspace not found"}`)
	})
	c := newTestClient(t, VendorInboxKit, fieldsFor(VendorInboxKit), srv.URL, nil)
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("Verify accepted an error envelope")
	}
}

func TestZapmail(t *testing.T) {
	page1 := `{"status":200,"message":"Mailboxes fetched successfully","data":{"totalSearchedCount":3,"currentPage":1,"nextPage":2,"totalPages":2,"purchasedMailboxes":500,"totalAssignedMailboxes":20,"totalActiveMailboxes":10,"availableMailboxes":480,"domains":[
		{"id":"12345abc-defg-6789-hijk-lmnopqrstu12","domain":"example1.com","status":"ACTIVE","adminDetails":{"domainId":"12345abc-defg-6789-hijk-lmnopqrstu12","email":"admin@example1.com","newPassword":"Wq7$nLpX2vTm","secret":"JBSWY3DPEHPK3PXP","recoveryEmail":"recovery.example1.com@example1.com"},
		 "mailboxes":[{"id":"abcd1234-5678-90ef-ghij-klmn12345678","username":"john.doe","email":"john.doe@example1.com","firstName":"John","lastName":"Doe","password":"password123","appPassword":"nfd2 jk43 4jas hfdj","secret":"gger u434","recoveryEmail":"john.doe@example1.com","status":"ACTIVE","profilePicture":null,"domain":"example1.com","domainId":"12345abc-defg-6789-hijk-lmnopqrstu12","assignedOn":"2025-01-10T09:00:00.000Z","expireOn":"2025-02-10T09:00:00.000Z","createdAt":"2025-01-10T09:00:00.000Z","isWarmedUp":false}]}]}}`
	page2 := `{"status":200,"message":"Mailboxes fetched successfully","data":{"totalSearchedCount":3,"currentPage":2,"nextPage":null,"totalPages":2,"domains":[
		{"id":"6789xyz-wxyz-4567-klmn-7890abcdef12","domain":"example2.net","status":"ACTIVE",
		 "mailboxes":[{"id":"xyz1234-5678-90ab-cdef-ghijk9876543","username":"jane.smith","email":"jane.smith@example2.net","firstName":"Jane","lastName":"Smith","password":"password456","appPassword":null,"secret":null,"recoveryEmail":"jane.smith@example2.net","status":"IN_PROGRESS","profilePicture":null,"domain":"example2.net","domainId":"6789xyz-wxyz-4567-klmn-7890abcdef12","assignedOn":"2025-01-12T11:00:00.000Z","expireOn":"2025-02-12T11:00:00.000Z","createdAt":"2025-01-12T11:00:00.000Z","isWarmedUp":false}]}]}}`
	msft := `{"status":200,"message":"Mailboxes fetched successfully","data":{"currentPage":1,"nextPage":null,"totalPages":1,"domains":[
		{"id":"d3","domain":"contoso.co","status":"ACTIVE","mailboxes":[{"id":"ms-1","username":"amy","email":"amy@contoso.co","firstName":"Amy","lastName":"Lee","password":"pw-ms","appPassword":null,"secret":null,"status":"ACTIVE","domain":"contoso.co"}]}]}}`
	srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users":
			writeJSON(w, 200, `{"status":200,"message":"ok","data":{}}`)
		case "/v2/mailboxes/list":
			switch {
			case r.Header.Get("x-service-provider") == "MICROSOFT":
				writeJSON(w, 200, msft)
			case r.URL.Query().Get("page") == "2":
				writeJSON(w, 200, page2)
			default:
				writeJSON(w, 200, page1)
			}
		case "/v2/mailboxes":
			if r.URL.Query().Get("id") == "missing" {
				writeJSON(w, 200, `{"status":404,"message":"Mailbox not found","errorId":"efb89401"}`)
				return
			}
			writeJSON(w, 200, `{"status":200,"message":"Mailbox fetched successfully","data":{"mailbox":{"id":"late-1","username":"late","firstName":"L","lastName":"T","status":"ACTIVE","profilePicture":null,"domain":"example1.com","appPassword":"late app","password":"nY8dfknx%6lCg*E9E","secret":"gger"}}}`)
		default:
			writeJSON(w, 404, `{}`)
		}
	})
	fields := map[string]string{FieldAPIKey: testKey, FieldWorkspaceID: "ws-123"}
	c := newTestClient(t, VendorZapmail, fields, srv.URL, nil)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	got := mustList(t, c)
	want := []Mailbox{
		{ID: "abcd1234-5678-90ef-ghij-klmn12345678", Email: "john.doe@example1.com", FirstName: "John", LastName: "Doe", Domain: "example1.com", Provider: ProviderGoogle, Status: "ACTIVE"},
		{ID: "xyz1234-5678-90ab-cdef-ghijk9876543", Email: "jane.smith@example2.net", FirstName: "Jane", LastName: "Smith", Domain: "example2.net", Provider: ProviderGoogle, Status: "IN_PROGRESS"},
		{ID: "ms-1", Email: "amy@contoso.co", FirstName: "Amy", LastName: "Lee", Domain: "contoso.co", Provider: ProviderMicrosoft, Status: "ACTIVE"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v\nwant %+v", got, want)
	}
	if cr := mustCreds(t, c, got[0]); cr != (Credentials{Password: "password123", AppPassword: "nfd2 jk43 4jas hfdj"}) {
		t.Fatalf("cached Credentials = %+v", cr)
	}
	if cr := mustCreds(t, c, got[1]); cr != (Credentials{Password: "password456"}) {
		t.Fatalf("null appPassword Credentials = %+v", cr)
	}
	listCalls := len(srv.requests())
	if cr := mustCreds(t, c, Mailbox{ID: "late-1", Provider: ProviderGoogle}); cr != (Credentials{Password: "nY8dfknx%6lCg*E9E", AppPassword: "late app"}) {
		t.Fatalf("fetched Credentials = %+v", cr)
	}
	if _, err := c.Credentials(context.Background(), Mailbox{ID: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status 404 in a 200 envelope: %v", err)
	}

	reqs := srv.requests()
	for _, r := range reqs {
		assertHeader(t, r, "x-auth-zapmail", testKey)
		assertHeader(t, r, "x-workspace-key", "ws-123")
		if r.Header.Get("Authorization") != "" {
			t.Errorf("%s sent an Authorization header", r.Path)
		}
	}
	var providers []string
	for _, r := range reqs[1:listCalls] {
		providers = append(providers, r.Header.Get("x-service-provider")+"/"+r.q("page"))
	}
	if !reflect.DeepEqual(providers, []string{"GOOGLE/1", "GOOGLE/2", "MICROSOFT/1"}) {
		t.Fatalf("list passes = %v", providers)
	}
	if reqs[listCalls].Query["id"][0] != "late-1" || reqs[listCalls].Header.Get("x-service-provider") != "GOOGLE" {
		t.Fatalf("detail request = %+v", reqs[listCalls])
	}
}

func TestZapmailForbiddenEnvelope(t *testing.T) {
	srv := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"status":403,"message":"You must be the owner or admin of this workspace to access this API","errorId":"463259c2"}`)
	})
	c := newTestClient(t, VendorZapmail, fieldsFor(VendorZapmail), srv.URL, nil)
	if _, err := c.List(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestZapmailSingleProviderNoWorkspace(t *testing.T) {
	srv := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"status":200,"data":{"currentPage":1,"totalPages":1,"domains":[]}}`)
	})
	c := newTestClient(t, VendorZapmail, map[string]string{FieldAPIKey: testKey, FieldServiceProvider: "microsoft"}, srv.URL, nil)
	if got := mustList(t, c); len(got) != 0 {
		t.Fatalf("List = %+v", got)
	}
	reqs := srv.requests()
	if len(reqs) != 1 || reqs[0].Header.Get("x-service-provider") != "MICROSOFT" || reqs[0].Header.Get("x-workspace-key") != "" {
		t.Fatalf("requests = %+v", reqs)
	}
}

// forgeBody is a Mailforge/Infraforge mailbox, shaped like the swagger definitions.
const forgeBody = `[
 {"createdAt":"2021-08-01T00:00:00Z","credentials":{"imapHost":"imap.server.com","imapPassword":"imap-password","imapPort":993,"imapUsername":"imap-username@example.com","smtpHost":"smtp.server.com","smtpPassword":"smtp-password","smtpPort":587,"smtpUsername":"smtp-username@example.com"},
  "domain":"example.com","email":"jondoe@example.com","firstName":"Jon","forwardingEmail":"forward_to@example.com","id":"mbx_1duj5a6534j37kzook2l9","isPrewarmed":false,"lastName":"Doe","setupId":"set_w3rz7eh5e1t12cvfhj1mv","signature":"Best, Jon Doe","status":"active","updatedAt":"2021-08-01T00:00:00Z","workspaceId":"wks_70my6ggvn5csfw3o27ojq"},
 {"credentials":{"imapHost":"mail.example.org","imapPassword":"same","imapPort":993,"imapUsername":"","smtpHost":"mail.example.org","smtpPassword":"same","smtpPort":465,"smtpUsername":""},
  "domain":"example.org","email":"amy@example.org","firstName":"Amy","id":"mbx_2","lastName":"Lee","status":"pending","workspaceId":"wks_2"}
]`

func TestForgeVendors(t *testing.T) {
	for _, vendor := range []string{VendorMailforge, VendorInfraforge} {
		t.Run(vendor, func(t *testing.T) {
			srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/workspaces":
					writeJSON(w, 200, `[{"accountId":"acc_1","id":"wks_70my6ggvn5csfw3o27ojq","name":"Main","slug":"main"}]`)
				case "/mailboxes":
					writeJSON(w, 200, forgeBody)
				case "/mailboxes/mbx_late":
					writeJSON(w, 200, `{"id":"mbx_late","email":"late@example.com","credentials":{"imapHost":"imap.x.com","imapPort":143,"imapPassword":"p","smtpHost":"smtp.x.com","smtpPort":587,"smtpPassword":"p"}}`)
				default:
					writeJSON(w, 404, `{"code":404,"message":"Mailbox not found"}`)
				}
			})
			fields := map[string]string{FieldAPIKey: testKey}
			if vendor == VendorInfraforge {
				fields[FieldWorkspaceID] = "wks_70my6ggvn5csfw3o27ojq"
			}
			c := newTestClient(t, vendor, fields, srv.URL, nil)
			if err := c.Verify(context.Background()); err != nil {
				t.Fatalf("Verify: %v", err)
			}
			got := mustList(t, c)
			want := []Mailbox{
				{ID: "mbx_1duj5a6534j37kzook2l9", Email: "jondoe@example.com", FirstName: "Jon", LastName: "Doe", Domain: "example.com", Provider: ProviderSMTP, Status: "active"},
				{ID: "mbx_2", Email: "amy@example.org", FirstName: "Amy", LastName: "Lee", Domain: "example.org", Provider: ProviderSMTP, Status: "pending"},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("List = %+v\nwant %+v", got, want)
			}
			cr := mustCreds(t, c, got[0])
			wantCr := Credentials{
				Password: "smtp-password",
				SMTP:     &Endpoint{Host: "smtp.server.com", Port: 587, Security: SecurityStartTLS, Username: "smtp-username@example.com"},
				IMAP:     &Endpoint{Host: "imap.server.com", Port: 993, Security: SecurityTLS, Username: "imap-username@example.com", Password: "imap-password"},
			}
			if !reflect.DeepEqual(cr, wantCr) {
				t.Fatalf("Credentials = %+v %+v %+v", cr, cr.SMTP, cr.IMAP)
			}
			cr2 := mustCreds(t, c, got[1])
			if cr2.Password != "same" || cr2.SMTP.Security != SecurityTLS || cr2.SMTP.Username != "amy@example.org" || cr2.IMAP.Password != "" {
				t.Fatalf("shared-password Credentials = %+v %+v %+v", cr2, cr2.SMTP, cr2.IMAP)
			}
			cr3 := mustCreds(t, c, Mailbox{ID: "mbx_late", Email: "late@example.com"})
			if cr3.Password != "p" || cr3.IMAP.Security != SecurityStartTLS {
				t.Fatalf("fetched Credentials = %+v %+v", cr3, cr3.IMAP)
			}
			if _, err := c.Credentials(context.Background(), Mailbox{ID: "mbx_gone"}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("gone: %v", err)
			}

			reqs := srv.requests()
			for _, r := range reqs {
				assertHeader(t, r, "Authorization", testKey)
			}
			list := reqs[1]
			if list.Query["with_credentials"][0] != "true" {
				t.Fatalf("list query = %v", list.Query)
			}
			if ws := list.q("workspace_id"); (vendor == VendorInfraforge) != (ws == "wks_70my6ggvn5csfw3o27ojq") {
				t.Fatalf("%s workspace_id = %q", vendor, ws)
			}
		})
	}
}

func maildosoAccountJSON(id int, email, provider string, hosted bool) string {
	imap, smtp := "null", "null"
	if hosted {
		imap = `{"imap_host":"imap.maildoso.com","port":993}`
		smtp = `{"smtp_host":"smtp.maildoso.com","port":587}`
	}
	return fmt.Sprintf(`{"id":%d,"user_id":7,"email_account":%q,"is_active":true,"domain_id":3,"vendor_uuid":null,"forwarding_account_id":null,"password":"pw-%d","first_name":"First%d","last_name":null,"picture_url":null,"is_premium_warmup":false,"created_at":"2025-01-01T00:00:00Z","imap":%s,"forwarding_account":null,"reputation_test":null,"provider":%q,"status":"active","is_admin":false,"exported_to_sequencers":[],"cancellation_at":null,"smtp":%s}`,
		id, email, id, id, imap, provider, smtp)
}

func TestMaildoso(t *testing.T) {
	srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/accounts-lookup" {
			writeJSON(w, 404, `{}`)
			return
		}
		q := r.URL.Query()
		switch {
		case q.Get("short") == "true":
			writeJSON(w, 200, `{"items":[{"id":1,"provider":"maildoso"}],"meta":{"offset":0,"limit":1,"total":3,"current":1}}`)
		case q.Get("keyword") != "":
			writeJSON(w, 200, `{"items":[`+maildosoAccountJSON(90, "late+x@d.com", "maildoso", true)+`,`+maildosoAccountJSON(91, "late@d.com", "maildoso", true)+`],"meta":{"offset":0,"limit":50,"total":2,"current":2}}`)
		case q.Get("offset") == "0":
			writeJSON(w, 200, `{"items":[`+maildosoAccountJSON(1, "ann@alpha.com", "maildoso", true)+`,`+maildosoAccountJSON(2, "bob@beta.com", "google", false)+`],"meta":{"offset":0,"limit":2,"total":3,"current":2}}`)
		default:
			writeJSON(w, 200, `{"items":[`+maildosoAccountJSON(3, "cy@gamma.com", "maildoso", true)+`],"meta":{"offset":2,"limit":2,"total":3,"current":1}}`)
		}
	})
	c := newTestClient(t, VendorMaildoso, fieldsFor(VendorMaildoso), srv.URL, nil)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	got := mustList(t, c)
	want := []Mailbox{
		{ID: "1", Email: "ann@alpha.com", FirstName: "First1", Domain: "alpha.com", Provider: ProviderSMTP, Status: "active"},
		{ID: "2", Email: "bob@beta.com", FirstName: "First2", Domain: "beta.com", Provider: ProviderGoogle, Status: "active"},
		{ID: "3", Email: "cy@gamma.com", FirstName: "First3", Domain: "gamma.com", Provider: ProviderSMTP, Status: "active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v\nwant %+v", got, want)
	}
	cr := mustCreds(t, c, got[0])
	wantCr := Credentials{
		Password: "pw-1",
		SMTP:     &Endpoint{Host: "smtp.maildoso.com", Port: 587, Security: SecurityStartTLS, Username: "ann@alpha.com"},
		IMAP:     &Endpoint{Host: "imap.maildoso.com", Port: 993, Security: SecurityTLS, Username: "ann@alpha.com"},
	}
	if !reflect.DeepEqual(cr, wantCr) {
		t.Fatalf("Credentials = %+v", cr)
	}
	if g := mustCreds(t, c, got[1]); g.SMTP != nil || g.IMAP != nil || g.Password != "pw-2" {
		t.Fatalf("google Credentials = %+v", g)
	}
	if late := mustCreds(t, c, Mailbox{ID: "91", Email: "late@d.com"}); late.Password != "pw-91" {
		t.Fatalf("lookup Credentials picked %+v", late)
	}

	reqs := srv.requests()
	for _, r := range reqs {
		assertHeader(t, r, "Authorization", "Bearer "+testKey)
	}
	if reqs[1].Query["offset"][0] != "0" || reqs[2].Query["offset"][0] != "2" {
		t.Fatalf("pagination offsets = %v, %v", reqs[1].Query, reqs[2].Query)
	}
}

func TestCheapInboxes(t *testing.T) {
	srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/org":
			writeJSON(w, 200, `{"organization":{"id":"550e8400-e29b-41d4-a716-446655440000","name":"Acme Outreach"},"role":"owner"}`)
		case "/v1/mailboxes":
			if r.URL.Query().Get("offset") == "0" {
				writeJSON(w, 200, `{"mailboxes":[{"id":"mb_abc123","domain_id":"d_abc123","full_email":"emma.johnson@acmeoutreach.com","first_name":"Emma","last_name":"Johnson","profile_picture_url":null,"status":"active","source_provider":"google","daily_limit":50,"tags":["campaign-1"],"target_integration_id":"int_abc123","created_at":"2025-06-15T10:00:00.000Z","activated_at":"2025-06-15T12:00:00.000Z"}],"pagination":{"total":2,"limit":1,"offset":0}}`)
				return
			}
			writeJSON(w, 200, `{"mailboxes":[{"id":"mb_def456","full_email":"liam@acme-mail.com","first_name":"Liam","last_name":"Ng","status":"provisioning","source_provider":"microsoft"}],"pagination":{"total":2,"limit":1,"offset":1}}`)
		case "/v1/mailboxes/mb_abc123/credentials":
			writeJSON(w, 200, `{"credentials":{"email":"emma.johnson@acmeoutreach.com","password":"SecureP@ss123!","app_password":"abcd efgh ijkl mnop","imap_host":"imap.gmail.com","imap_port":993,"smtp_host":"smtp.gmail.com","smtp_port":587}}`)
		default:
			writeJSON(w, 404, `{"error":{"code":"NOT_FOUND","message":"not found"}}`)
		}
	})
	s := &sleeps{}
	c := newTestClient(t, VendorCheapInboxes, fieldsFor(VendorCheapInboxes), srv.URL, s)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	got := mustList(t, c)
	want := []Mailbox{
		{ID: "mb_abc123", Email: "emma.johnson@acmeoutreach.com", FirstName: "Emma", LastName: "Johnson", Domain: "acmeoutreach.com", Provider: ProviderGoogle, Status: "active"},
		{ID: "mb_def456", Email: "liam@acme-mail.com", FirstName: "Liam", LastName: "Ng", Domain: "acme-mail.com", Provider: ProviderMicrosoft, Status: "provisioning"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v\nwant %+v", got, want)
	}
	cr := mustCreds(t, c, got[0])
	wantCr := Credentials{
		Password:    "SecureP@ss123!",
		AppPassword: "abcd efgh ijkl mnop",
		SMTP:        &Endpoint{Host: "smtp.gmail.com", Port: 587, Security: SecurityStartTLS, Username: "emma.johnson@acmeoutreach.com"},
		IMAP:        &Endpoint{Host: "imap.gmail.com", Port: 993, Security: SecurityTLS, Username: "emma.johnson@acmeoutreach.com"},
	}
	if !reflect.DeepEqual(cr, wantCr) {
		t.Fatalf("Credentials = %+v", cr)
	}
	if _, err := c.Credentials(context.Background(), got[1]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing credentials: %v", err)
	}
	reqs := srv.requests()
	for _, r := range reqs {
		assertHeader(t, r, "Authorization", "Bearer "+testKey)
	}
	if reqs[1].Query["offset"][0] != "0" || reqs[2].Query["offset"][0] != "1" {
		t.Fatalf("pagination = %v, %v", reqs[1].Query, reqs[2].Query)
	}
	if len(s.all()) == 0 {
		t.Fatal("Cheap Inboxes calls were not throttled")
	}
}

func TestScaledMail(t *testing.T) {
	const org = "recORG000000001"
	srv := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/organizations":
			writeJSON(w, 200, `{"organizations":[{"id":"recORG000000001"}]}`)
		case "/domains":
			writeJSON(w, 200, `{"total":2,"domains":[
				{"id":"recDOM1","domain":"outreach-one.com","tag":"campaign-q3","redirect":"https://example.com","order_type":"google","order_id":"recORD1","payment_id":"recPAY1","domain_provider":"Scaledmail","user_id":"recUSR1","total_mailboxes":2,"mailbox":[{"first_name":"Jane","last_name":"Doe","alias":"jane"}],"status":"Active"},
				{"id":"recDOM2","domain":"outreach-two.com","order_type":"outlook","total_mailboxes":1,"mailbox":[],"status":"Active"}]}`)
		case "/mailboxes/recDOM1":
			writeJSON(w, 200, `{"total":2,"mailboxes":[
				{"first_name":"Jane","last_name":"Doe","alias":"jane","domain_id":"recDOM1","payment_id":"recPAY1","status":"Active","tag":"campaign-q3","mailbox_password":"Sup3rSecret!","email":"jane@outreach-one.com","order_type":"google","order_id":"recORD1"},
				{"first_name":"Joe","last_name":"Roe","alias":"joe","domain_id":"recDOM1","status":"Active","mailbox_password":"An0ther!","order_type":"google"}]}`)
		case "/mailboxes/recDOM2":
			writeJSON(w, 200, `{"total":1,"mailboxes":[{"first_name":"Kim","last_name":"Park","alias":"kim","domain_id":"recDOM2","status":"Active","mailbox_password":"Outl00k!","email":"kim@outreach-two.com","order_type":"outlook"}]}`)
		default:
			writeJSON(w, 404, `{"message":"Unable to get mailboxes or domain ID does not exist"}`)
		}
	})
	s := &sleeps{}
	c := newTestClient(t, VendorScaledMail, map[string]string{FieldAPIKey: testKey, FieldOrganizationID: org}, srv.URL, s)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	got := mustList(t, c)
	want := []Mailbox{
		{ID: "jane@outreach-one.com", Email: "jane@outreach-one.com", FirstName: "Jane", LastName: "Doe", Domain: "outreach-one.com", Provider: ProviderGoogle, Status: "Active"},
		{ID: "joe@outreach-one.com", Email: "joe@outreach-one.com", FirstName: "Joe", LastName: "Roe", Domain: "outreach-one.com", Provider: ProviderGoogle, Status: "Active"},
		{ID: "kim@outreach-two.com", Email: "kim@outreach-two.com", FirstName: "Kim", LastName: "Park", Domain: "outreach-two.com", Provider: ProviderMicrosoft, Status: "Active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v\nwant %+v", got, want)
	}
	if cr := mustCreds(t, c, got[0]); cr != (Credentials{Password: "Sup3rSecret!"}) {
		t.Fatalf("Credentials = %+v", cr)
	}

	// A fresh client has no cache, so it re-reads the mailbox's domain.
	fresh := newTestClient(t, VendorScaledMail, map[string]string{FieldAPIKey: testKey, FieldOrganizationID: org}, srv.URL, nil)
	if cr := mustCreds(t, fresh, Mailbox{ID: "kim@outreach-two.com", Email: "kim@outreach-two.com", Domain: "outreach-two.com"}); cr.Password != "Outl00k!" {
		t.Fatalf("fetched Credentials = %+v", cr)
	}
	if _, err := fresh.Credentials(context.Background(), Mailbox{Email: "nobody@outreach-two.com"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown mailbox: %v", err)
	}

	for _, r := range srv.requests() {
		assertHeader(t, r, "Authorization", "Bearer "+testKey)
		if r.Path == "/organizations" {
			continue
		}
		if r.q("organization_id") != org {
			t.Errorf("%s: organization_id = %q", r.Path, r.q("organization_id"))
		}
		if strings.HasPrefix(r.Path, "/mailboxes/") && r.q("password") != "true" {
			t.Errorf("%s: password=true missing", r.Path)
		}
	}
	// Four calls in a row are spaced at the documented 5 per second.
	waits := s.all()
	if len(waits) < 3 {
		t.Fatalf("waits = %v, want the throttle to space calls", waits)
	}
	for _, w := range waits {
		if w > time.Second {
			t.Fatalf("throttle wait %v is longer than a second", w)
		}
	}
}

func TestListCapsAtMax(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := range MaxMailboxes + 5 {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":"mbx_%d","email":"u%d@x.com","status":"active"}`, i, i)
	}
	sb.WriteString("]")
	body := sb.String()
	srv := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, body) })
	c := newTestClient(t, VendorMailforge, fieldsFor(VendorMailforge), srv.URL, nil)
	if got := mustList(t, c); len(got) != MaxMailboxes {
		t.Fatalf("List returned %d, want %d", len(got), MaxMailboxes)
	}
}
