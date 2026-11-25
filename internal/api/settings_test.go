package api_test

import (
	"strings"
	"testing"
)

func setupOwner(t *testing.T, newClient func() *client) *client {
	t.Helper()
	c := newClient()
	if code, body, _ := c.do("POST", "/api/0/setup", map[string]string{
		"name": "Owner", "email": "owner@example.com", "password": pw, "organizationName": "Acme"}); code != 201 {
		t.Fatalf("setup = %d %v", code, body)
	}
	return c
}

func TestProjectSettingsAndKeys(t *testing.T) {
	_, newClient := newEnv(t)
	c := setupOwner(t, newClient)
	c.do("POST", "/api/0/organizations/acme/projects", map[string]string{"name": "Web", "platform": "javascript"})

	code, ps, _ := c.do("GET", "/api/0/organizations/acme/projects/web/", nil)
	if code != 200 || ps["scrubIp"] != "truncate" || ps["scrubDefaults"] != true || len(ps["keys"].([]any)) != 1 {
		t.Fatalf("settings = %d %v", code, ps)
	}
	code, body, _ := c.do("PUT", "/api/0/organizations/acme/projects/web/", map[string]any{
		"name": "Web App", "platform": "javascript", "retentionDays": 14, "scrubDefaults": true,
		"scrubIp": "drop", "sensitiveFields": []string{"national_id"}, "safeFields": []string{}})
	if code != 200 {
		t.Fatalf("update = %d %v", code, body)
	}
	_, ps, _ = c.do("GET", "/api/0/organizations/acme/projects/web/", nil)
	if ps["scrubIp"] != "drop" || ps["retentionDays"].(float64) != 14 || ps["name"] != "Web App" {
		t.Fatalf("not saved: %v", ps)
	}
	if code, _, _ := c.do("PUT", "/api/0/organizations/acme/projects/web/", map[string]any{
		"name": "x", "platform": "javascript", "retentionDays": 0, "scrubIp": "drop"}); code != 400 {
		t.Fatalf("invalid retention accepted: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/0/organizations/acme/projects/web/keys", map[string]string{"label": "CI"}); code != 200 {
		t.Fatalf("create key = %d", code)
	}
	_, ps, _ = c.do("GET", "/api/0/organizations/acme/projects/web/", nil)
	keys := ps["keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("keys = %v", keys)
	}
	id := keys[0].(map[string]any)["id"].(string)
	if code, _, _ := c.do("PUT", "/api/0/organizations/acme/projects/web/keys/"+id, map[string]bool{"active": false}); code != 200 {
		t.Fatalf("disable key = %d", code)
	}
	if code, _, _ := c.do("DELETE", "/api/0/organizations/acme/projects/web/", nil); code != 204 {
		t.Fatalf("delete project = %d", code)
	}
	if _, _, list := c.do("GET", "/api/0/organizations/acme/projects", nil); len(list) != 0 {
		t.Fatalf("project not deleted: %v", list)
	}
}

func TestInviteFlowAndRoles(t *testing.T) {
	_, newClient := newEnv(t)
	owner := setupOwner(t, newClient)

	code, inv, _ := owner.do("POST", "/api/0/organizations/acme/members", map[string]string{"email": "dev@example.com", "role": "member"})
	if code != 201 || inv["emailed"] != false {
		t.Fatalf("invite = %d %v", code, inv)
	}
	link := inv["link"].(string)
	token := link[strings.LastIndex(link, "/")+1:]

	dev := newClient()
	code, info, _ := dev.do("GET", "/api/0/invitations/"+token, nil)
	if code != 200 || info["email"] != "dev@example.com" || info["accountExists"] != false {
		t.Fatalf("invite info = %d %v", code, info)
	}
	if code, _, _ := dev.do("POST", "/api/0/invitations/"+token+"/accept", map[string]string{"name": "Dev", "password": "short"}); code != 400 {
		t.Fatalf("weak password accepted: %d", code)
	}
	if code, body, _ := dev.do("POST", "/api/0/invitations/"+token+"/accept", map[string]string{"name": "Dev", "password": pw}); code != 200 {
		t.Fatalf("accept = %d %v", code, body)
	}
	if code, _, _ := dev.do("GET", "/api/0/invitations/"+token, nil); code != 404 {
		t.Fatalf("token reusable: %d", code)
	}
	// Member can view issues but not admin endpoints.
	if code, _, _ := dev.do("GET", "/api/0/organizations/acme/issues", nil); code != 200 {
		t.Fatalf("member issues = %d", code)
	}
	if code, _, _ := dev.do("GET", "/api/0/organizations/acme/alerts/rules", nil); code != 403 {
		t.Fatalf("member admin access = %d", code)
	}
	if code, _, _ := dev.do("POST", "/api/0/organizations/acme/members", map[string]string{"email": "x@example.com", "role": "member"}); code != 403 {
		t.Fatalf("member could invite: %d", code)
	}

	_, members, _ := owner.do("GET", "/api/0/organizations/acme/members", nil)
	var ownerID, devID string
	for _, m := range members["members"].([]any) {
		mm := m.(map[string]any)
		if mm["email"] == "owner@example.com" {
			ownerID = mm["userId"].(string)
		} else {
			devID = mm["userId"].(string)
		}
	}
	if code, body, _ := owner.do("PUT", "/api/0/organizations/acme/members/"+ownerID, map[string]string{"role": "member"}); code != 400 {
		t.Fatalf("last owner demoted: %d %v", code, body)
	}
	if code, _, _ := owner.do("DELETE", "/api/0/organizations/acme/members/"+devID, nil); code != 204 {
		t.Fatalf("remove member = %d", code)
	}
	if code, _, _ := dev.do("GET", "/api/0/organizations/acme/issues", nil); code != 404 {
		t.Fatalf("removed member still has access: %d", code)
	}
}

func TestAlertConfigValidation(t *testing.T) {
	_, newClient := newEnv(t)
	c := setupOwner(t, newClient)
	if code, _, _ := c.do("POST", "/api/0/organizations/acme/alerts/channels", map[string]any{"kind": "slack", "name": "x", "url": "not a url"}); code != 400 {
		t.Fatalf("bad url accepted: %d", code)
	}
	code, ch, _ := c.do("POST", "/api/0/organizations/acme/alerts/channels", map[string]any{"kind": "webhook", "name": "Ops", "url": "https://hooks.example.com/abc/secret-path", "secret": "k"})
	if code != 201 {
		t.Fatalf("create channel = %d", code)
	}
	_, _, list := c.do("GET", "/api/0/organizations/acme/alerts/channels", nil)
	got := list[0].(map[string]any)
	if got["target"] != "https://hooks.example.com/…" || got["hasSecret"] != true {
		t.Fatalf("channel not masked: %v", got)
	}
	if code, _, _ := c.do("POST", "/api/0/organizations/acme/alerts/rules", map[string]any{"name": "spike", "trigger": "frequency", "threshold": 0, "channelIds": []string{ch["id"].(string)}}); code != 400 {
		t.Fatalf("bad frequency rule accepted: %d", code)
	}
	if code, body, _ := c.do("POST", "/api/0/organizations/acme/alerts/rules", map[string]any{"name": "new", "trigger": "new_issue", "channelIds": []string{ch["id"].(string)}}); code != 201 {
		t.Fatalf("create rule = %d %v", code, body)
	}
	if code, _, _ := c.do("POST", "/api/0/organizations/acme/alerts/rules", map[string]any{"name": "x", "trigger": "new_issue", "channelIds": []string{"00000000-0000-0000-0000-000000000000"}}); code != 400 {
		t.Fatalf("unknown channel accepted: %d", code)
	}
}
