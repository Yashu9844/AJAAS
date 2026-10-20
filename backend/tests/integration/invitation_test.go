package integration

import "testing"

// Invitation lifecycle (M0 plan B3): invite → token by the secret channel → accept → sign in; resend, revoke,
// single use, escalation.

func mailboxToken(t *testing.T, tc *tenantCtx, email, kind string) string {
	t.Helper()
	box := call(t, "GET", "/dev/mailbox", platform(), query("tenant_id", tc.id, "email", email))
	expect(t, box, 200, "dev mailbox")
	for _, it := range box.list("data") {
		m := it.(map[string]any)
		if m["kind"] == kind {
			return m["token"].(string)
		}
	}
	t.Fatalf("no %s message for %s: %s", kind, email, box.Raw)
	return ""
}

func TestInvitationLifecycle(t *testing.T) {
	tc := newTenant(t)
	email := "invitee." + uniq() + "@" + tc.slug + ".test"
	inv := tc.do(t, "POST", "/invitations", body(map[string]any{"email": email, "first_name": "In", "last_name": "Vitee"}))
	expect(t, inv, 201, "invite")
	if inv.str("data.user.status") != "invited" || inv.str("data.invitation.status") != "pending" {
		t.Fatalf("invited account with a pending invitation: %s", inv.Raw)
	}
	first := mailboxToken(t, tc, email, "invitation")
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": email, "password": password})), 401, "", "no login before accepting")

	// resend revokes the first token
	re := tc.do(t, "POST", "/invitations/"+inv.str("data.invitation.id")+"/resend")
	expect(t, re, 200, "resend")
	second := mailboxToken(t, tc, email, "invitation")
	if second == first {
		t.Fatal("resend issues a new token")
	}
	accept := func(tok string) resp {
		return call(t, "POST", "/auth/accept-invitation", body(map[string]string{"token": tok, "password": password, "confirm_password": password}))
	}
	expectErr(t, accept(first), 400, "INVALID_TOKEN", "old token is dead")
	expect(t, accept(second), 200, "accept")
	expectErr(t, accept(second), 400, "INVALID_TOKEN", "single use")
	login(t, tc.slug, email, password)
	if n := sqlStr(t, "SELECT count(*) FROM identity_events_outbox WHERE tenant_id = ? AND routing_key = 'identity.user.activated'", tc.id); n != "1" {
		t.Fatalf("activation event: %s", n)
	}
	list := tc.do(t, "GET", "/invitations")
	if !list.has("data", "status", "accepted") || !list.has("data", "status", "revoked") {
		t.Fatalf("history lists accepted and revoked invitations: %s", list.Raw)
	}
	if n := len(tc.do(t, "GET", "/invitations", query("status", "pending")).list("data")); n != 0 {
		t.Fatalf("no pending invitations left: %d", n)
	}
}

func TestInvitationRevokeAndEscalation(t *testing.T) {
	tc := newTenant(t)
	email := "rev." + uniq() + "@" + tc.slug + ".test"
	inv := tc.do(t, "POST", "/invitations", body(map[string]any{"email": email, "first_name": "R", "last_name": "V"}))
	expect(t, inv, 201, "invite")
	tok := mailboxToken(t, tc, email, "invitation")
	expect(t, tc.do(t, "POST", "/invitations/"+inv.str("data.invitation.id")+"/revoke"), 200, "revoke")
	expectErr(t, call(t, "POST", "/auth/accept-invitation", body(map[string]string{"token": tok, "password": password, "confirm_password": password})), 400, "INVALID_TOKEN", "revoked")
	expectErr(t, tc.do(t, "POST", "/invitations", body(map[string]any{"email": email, "first_name": "R", "last_name": "V"})), 409, "CONFLICT", "email taken")

	// an inviter with users:create only cannot hand out a stronger role
	strong := tc.do(t, "POST", "/roles", body(map[string]any{"name": "strong-" + uniq(), "permission_ids": []string{tc.permID(t, "payroll", "approve")}}))
	expect(t, strong, 201, "strong role")
	limTok, _, _ := tc.limitedUser(t, [2]string{"users", "create"})
	r := call(t, "POST", "/invitations", host(tc.slug), token(limTok), body(map[string]any{
		"email": "esc." + uniq() + "@x.test", "first_name": "E", "last_name": "S", "role_ids": []string{strong.str("data.id")}}))
	expectErr(t, r, 403, "FORBIDDEN", "invite with a stronger role")
}
