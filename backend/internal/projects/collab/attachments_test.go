package collab

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/storage"
)

// upload runs init and plays the client's upload, returning the pending attachment.
func (f *commentFixture) upload(who api.Actor, task uuid.UUID, name string, data []byte) *Attachment {
	f.t.Helper()
	tk, err := f.svc.InitAttachment(bg, who, task, InitInput{Filename: name, Size: int64(len(data)), ContentType: "text/plain"})
	must(f.t, err)
	f.mem.Put(tk.Attachment.StorageKey, "text/plain", data)
	return tk.Attachment
}

func TestAttachmentHappyPath(t *testing.T) {
	f := newCommentFixture(t)
	secretName := "SECRET-REPORT-NAME.txt"
	data := []byte("hello attachments")
	tk, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: secretName, Size: int64(len(data)), ContentType: "Text/Plain; charset=utf-8"})
	must(t, err)
	att := tk.Attachment
	wantKey := "t/" + f.tenant.String() + "/p/" + f.pid.String() + "/a/" + att.ID.String()
	if att.StorageKey != wantKey || att.Status != "pending" || att.ContentType != "text/plain" || tk.Upload.Method != "PUT" {
		t.Fatalf("init: %+v %+v", att, tk.Upload)
	}
	if time.Until(tk.Upload.Expires) > UploadTTL || time.Until(tk.Upload.Expires) < UploadTTL-time.Minute {
		t.Fatalf("upload ttl: %v", tk.Upload.Expires)
	}
	// Pending uploads are not listed and cannot be downloaded.
	if rows, _, _ := f.svc.ListAttachments(bg, f.member, f.task, PageRequest{}); len(rows) != 0 {
		t.Fatalf("pending listed: %v", rows)
	}
	_, err = f.svc.DownloadAttachment(bg, f.member, att.ID)
	wantStatus(t, err, "404")
	f.mem.Put(att.StorageKey, "text/plain", data)
	done, err := f.svc.CompleteAttachment(bg, f.member, att.ID)
	must(t, err)
	if done.Status != "ready" || done.ReadyAt == nil {
		t.Fatalf("complete: %+v", done)
	}
	again, err := f.svc.CompleteAttachment(bg, f.member, att.ID) // idempotent
	must(t, err)
	if again.Status != "ready" || len(f.events("attachment.added")) != 1 {
		t.Fatal("completing twice announces once")
	}
	ev := f.events("attachment.added")[0]
	if ev["attachment_id"] != att.ID.String() || ev["task_id"] != f.task.String() || ev["project_id"] != f.pid.String() {
		t.Fatalf("payload: %v", ev)
	}
	f.noLeak(secretName, "SECRET-REPORT")
	if f.count(`SELECT count(*) FROM activity WHERE task_id = ? AND type = 'attached'`, f.task) != 1 {
		t.Fatal("attached activity missing")
	}
	rows, _, err := f.svc.ListAttachments(bg, f.viewer, f.task, PageRequest{})
	must(t, err)
	if len(rows) != 1 || rows[0].Filename != secretName {
		t.Fatalf("list: %+v", rows)
	}
	link, err := f.svc.DownloadAttachment(bg, f.viewer, att.ID)
	must(t, err)
	if !strings.Contains(link.URL, att.ID.String()) || link.Filename != secretName || time.Until(link.ExpiresAt) > DownloadTTL {
		t.Fatalf("link: %+v", link)
	}
	// AT-03: downloads from a private project are logged, from a tenant-visible one they are not.
	if f.count(`SELECT count(*) FROM activity WHERE type = 'attachment_downloaded' AND task_id = ?`, f.task) != 1 {
		t.Fatal("private download not logged")
	}
}

func TestDownloadACL(t *testing.T) {
	f := newCommentFixture(t)
	att := f.upload(f.member, f.task, "a.txt", []byte("abc"))
	_, err := f.svc.CompleteAttachment(bg, f.member, att.ID)
	must(t, err)
	// PG8: a user without project access, a user of another tenant, unknown ids and removed members get 404.
	_, err = f.svc.DownloadAttachment(bg, f.stranger, att.ID)
	wantStatus(t, err, "404")
	foreign := f.actor()
	foreign.TenantID = uuid.New()
	_, err = f.svc.DownloadAttachment(bg, foreign, att.ID)
	wantStatus(t, err, "404")
	_, err = f.svc.DownloadAttachment(bg, f.owner, uuid.New())
	wantStatus(t, err, "404")
	must(t, f.proj.RemoveMember(bg, f.owner, f.pid, f.viewer.UserID))
	_, err = f.svc.DownloadAttachment(bg, f.viewer, att.ID)
	wantStatus(t, err, "404")
	if _, err = f.svc.DownloadAttachment(bg, f.owner, att.ID); err != nil {
		t.Fatalf("members download: %v", err)
	}
	// Deleted attachments disappear.
	must(t, f.svc.DeleteAttachment(bg, f.member, att.ID))
	_, err = f.svc.DownloadAttachment(bg, f.owner, att.ID)
	wantStatus(t, err, "404")
}

func TestTenantVisibleProjectDownloadsAreNotLogged(t *testing.T) {
	e := newEnv(t)
	owner, reader := e.actor(), e.actor()
	p := e.project(owner, "PUB", "tenant")
	task := e.task(p, nil)
	f := &commentFixture{env: e, owner: owner, p: p, pid: p.ID, task: task}
	att := f.upload(owner, task, "a.txt", []byte("abc"))
	_, err := e.svc.CompleteAttachment(bg, owner, att.ID)
	must(t, err)
	// A reader of a tenant-visible project may download but never upload (attachment.upload).
	if _, err = e.svc.DownloadAttachment(bg, reader, att.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.InitAttachment(bg, reader, task, InitInput{Filename: "x.txt", Size: 1, ContentType: "text/plain"})
	wantStatus(t, err, "403")
	if e.count(`SELECT count(*) FROM activity WHERE type = 'attachment_downloaded'`) != 0 {
		t.Fatal("tenant-visible downloads are not logged")
	}
}

func TestInitValidationTable(t *testing.T) {
	f := newCommentFixture(t)
	ok := InitInput{Filename: "notes.txt", Size: 10, ContentType: "text/plain"}
	with := func(fn func(*InitInput)) InitInput { in := ok; fn(&in); return in }
	cases := []struct {
		name string
		in   InitInput
		code string
	}{
		{"zero size", with(func(i *InitInput) { i.Size = 0 }), "VALIDATION_ERROR"},
		{"over 50 MB", with(func(i *InitInput) { i.Size = MaxFileBytes + 1 }), "FILE_TOO_LARGE"},
		{"html", with(func(i *InitInput) { i.ContentType = "text/html" }), "FILE_TYPE_NOT_ALLOWED"},
		{"svg", with(func(i *InitInput) { i.ContentType = "image/svg+xml" }), "FILE_TYPE_NOT_ALLOWED"},
		{"octet-stream", with(func(i *InitInput) { i.ContentType = "application/octet-stream" }), "FILE_TYPE_NOT_ALLOWED"},
		{"exe extension", with(func(i *InitInput) { i.Filename = "setup.EXE" }), "FILE_TYPE_NOT_ALLOWED"},
		{"double extension", with(func(i *InitInput) { i.Filename = "invoice.pdf.bat" }), "FILE_TYPE_NOT_ALLOWED"},
		{"hidden extension", with(func(i *InitInput) { i.Filename = "run.sh.txt" }), "FILE_TYPE_NOT_ALLOWED"},
		{"empty name", with(func(i *InitInput) { i.Filename = "" }), "VALIDATION_ERROR"},
		{"dots only", with(func(i *InitInput) { i.Filename = " .. " }), "VALIDATION_ERROR"},
		{"slashes only", with(func(i *InitInput) { i.Filename = "a/b/" }), "VALIDATION_ERROR"},
		{"long name", with(func(i *InitInput) { i.Filename = strings.Repeat("a", 252) + ".txt" }), "VALIDATION_ERROR"},
		{"bad utf8", with(func(i *InitInput) { i.Filename = "a\xffb.txt" }), "VALIDATION_ERROR"},
		{"bad sha", with(func(i *InitInput) { i.SHA256 = "xyz" }), "VALIDATION_ERROR"},
	}
	for _, c := range cases {
		_, err := f.svc.InitAttachment(bg, f.member, f.task, c.in)
		if err == nil || !strings.Contains(err.Error(), c.code) || !strings.Contains(err.Error(), "status: 400") {
			t.Errorf("%s: want %s/400, got %v", c.name, c.code, err)
		}
	}
	if f.count(`SELECT count(*) FROM attachments`) != 0 {
		t.Fatal("refused inits must not leave rows")
	}
	// A path in the name is cut to its last segment; control and bidi characters are removed.
	tk, err := f.svc.InitAttachment(bg, f.member, f.task, with(func(i *InitInput) { i.Filename = "../../etc/pass‮wd\x07.txt" }))
	must(t, err)
	if tk.Attachment.Filename != "passwd.txt" {
		t.Fatalf("sanitised: %q", tk.Attachment.Filename)
	}
	long := strings.Repeat("é", 251) + ".txt" // 255 characters, more than 255 bytes
	if _, err = f.svc.InitAttachment(bg, f.member, f.task, with(func(i *InitInput) { i.Filename = long })); err != nil {
		t.Fatalf("255 characters are fine: %v", err)
	}
	// Permissions and unknown ids.
	_, err = f.svc.InitAttachment(bg, f.viewer, f.task, ok)
	wantStatus(t, err, "403")
	_, err = f.svc.InitAttachment(bg, f.stranger, f.task, ok)
	wantStatus(t, err, "404")
	_, err = f.svc.InitAttachment(bg, f.member, uuid.New(), ok)
	wantStatus(t, err, "404")
}

func TestQuotaAndTaskLimit(t *testing.T) {
	f := newCommentFixture(t)
	in := InitInput{Filename: "a.txt", Size: 1024, ContentType: "text/plain"}
	f.db.Exec(`INSERT INTO attachments (id, tenant_id, project_id, task_id, storage_key, filename, content_type, size, status, uploaded_by)
		VALUES (?, ?, ?, ?, 'k', 'big.txt', 'text/plain', ?, 'ready', ?)`, uuid.New(), f.tenant, f.pid, f.task, int64(TenantQuotaBytes-512), f.owner.UserID)
	_, err := f.svc.InitAttachment(bg, f.member, f.task, in)
	wantCode(t, err, "QUOTA_EXCEEDED")
	f.db.Exec(`UPDATE attachments SET size = 1`)
	if _, err = f.svc.InitAttachment(bg, f.member, f.task, in); err != nil {
		t.Fatalf("within quota: %v", err)
	}
	// A deleted row frees its quota; another task is unaffected by this task's limit.
	f.db.Exec(`UPDATE attachments SET status = 'deleted', size = ?`, int64(TenantQuotaBytes))
	if _, err = f.svc.InitAttachment(bg, f.member, f.task, in); err != nil {
		t.Fatalf("deleted rows do not count: %v", err)
	}
	f.db.Exec(`DELETE FROM attachments`)
	f.db.Exec(`INSERT INTO attachments (id, tenant_id, project_id, task_id, storage_key, filename, content_type, size, status, uploaded_by)
		SELECT gen_random_uuid(), ?, ?, ?, 'k', 'f.txt', 'text/plain', 1, 'ready', ? FROM generate_series(1, ?)`, f.tenant, f.pid, f.task, f.owner.UserID, MaxPerTask)
	_, err = f.svc.InitAttachment(bg, f.member, f.task, in)
	wantCode(t, err, "TOO_MANY_ATTACHMENTS")
	if _, err = f.svc.InitAttachment(bg, f.member, f.env.task(f.p, nil), in); err != nil {
		t.Fatalf("limit is per task: %v", err)
	}
}

func TestCompleteMismatchKeepsPending(t *testing.T) {
	f := newCommentFixture(t)
	data := []byte("12345")
	sum := "a4e624d686e03ed2767c0abd85c14426b0b1157d2ce81d27bb4fe4f6f01d688a" // not the sha of data on purpose
	tk, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "a.txt", Size: 5, ContentType: "text/plain", SHA256: strings.ToUpper(sum)})
	must(t, err)
	key := tk.Attachment.StorageKey
	_, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	wantCode(t, err, "UPLOAD_MISMATCH") // nothing uploaded yet
	f.mem.Put(key, "text/plain", []byte("too long data"))
	_, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	wantCode(t, err, "UPLOAD_MISMATCH") // size
	f.mem.Put(key, "image/png", data)
	_, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	wantCode(t, err, "UPLOAD_MISMATCH") // type
	f.mem.Put(key, "text/plain", data)
	_, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	wantCode(t, err, "UPLOAD_MISMATCH") // checksum
	var status string
	f.db.Raw(`SELECT status FROM attachments WHERE id = ?`, tk.Attachment.ID).Scan(&status)
	if status != "pending" || len(f.events("attachment.added")) != 0 {
		t.Fatalf("a mismatch leaves the row pending: %s", status)
	}
	// Without a declared checksum the same bytes complete.
	ok := f.upload(f.member, f.task, "b.txt", data)
	if _, err = f.svc.CompleteAttachment(bg, f.member, ok.ID); err != nil {
		t.Fatal(err)
	}
}

func TestChecksumAcceptedWhenItMatches(t *testing.T) {
	f := newCommentFixture(t)
	data := []byte("checksum me")
	tmp := storage.NewMemory()
	tmp.Put("k", "text/plain", data)
	info, _ := tmp.Head(bg, "k")
	tk, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "c.txt", Size: int64(len(data)), ContentType: "text/plain", SHA256: info.SHA256})
	must(t, err)
	f.mem.Put(tk.Attachment.StorageKey, "text/plain", data)
	if _, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCompletePermissionsAndPendingVisibility(t *testing.T) {
	f := newCommentFixture(t)
	att := f.upload(f.member, f.task, "a.txt", []byte("abc"))
	other := f.actor()
	f.join(f.owner, f.p, other, "member")
	_, err := f.svc.CompleteAttachment(bg, other, att.ID)
	wantStatus(t, err, "404") // pending uploads exist only for their uploader
	wantStatus(t, f.svc.DeleteAttachment(bg, other, att.ID), "404")
	_, err = f.svc.CompleteAttachment(bg, f.stranger, att.ID)
	wantStatus(t, err, "404")
	_, err = f.svc.CompleteAttachment(bg, f.member, uuid.New())
	wantStatus(t, err, "404")
	_, err = f.svc.CompleteAttachment(bg, f.member, att.ID)
	must(t, err)
	_, err = f.svc.CompleteAttachment(bg, other, att.ID)
	wantStatus(t, err, "403") // ready, but not theirs
	// The project can be archived between init and complete.
	pending := f.upload(f.member, f.task, "b.txt", []byte("abc"))
	_, err = f.proj.Archive(bg, f.owner, f.pid)
	must(t, err)
	_, err = f.svc.CompleteAttachment(bg, f.member, pending.ID)
	wantCode(t, err, "PROJECT_ARCHIVED")
	_, err = f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "c.txt", Size: 1, ContentType: "text/plain"})
	wantCode(t, err, "PROJECT_ARCHIVED")
}

type verdictScanner struct {
	clean bool
	err   error
}

func (v verdictScanner) Scan(context.Context, string, storage.ObjectInfo) (bool, error) {
	return v.clean, v.err
}

func TestMalwareHook(t *testing.T) {
	f := newCommentFixture(t)
	f.svc.p.Scanner = verdictScanner{err: errBoom}
	att := f.upload(f.member, f.task, "a.txt", []byte("abc"))
	_, err := f.svc.CompleteAttachment(bg, f.member, att.ID)
	wantCode(t, err, "SCAN_UNAVAILABLE")
	var status string
	f.db.Raw(`SELECT status FROM attachments WHERE id = ?`, att.ID).Scan(&status)
	if status != "pending" {
		t.Fatalf("a scanner outage keeps the upload pending: %s", status)
	}
	f.svc.p.Scanner = verdictScanner{clean: false}
	_, err = f.svc.CompleteAttachment(bg, f.member, att.ID)
	wantCode(t, err, "MALWARE_DETECTED")
	if f.mem.Has(att.StorageKey) {
		t.Fatal("an infected object is removed")
	}
	f.db.Raw(`SELECT status FROM attachments WHERE id = ?`, att.ID).Scan(&status)
	if status != "deleted" {
		t.Fatalf("infected upload: %s", status)
	}
	f.svc.p.Scanner = NoopScanner{}
	clean, _ := NoopScanner{}.Scan(bg, "k", storage.ObjectInfo{})
	if !clean {
		t.Fatal("the default scanner accepts everything")
	}
}

func TestDeleteAttachmentRules(t *testing.T) {
	f := newCommentFixture(t)
	other := f.actor()
	f.join(f.owner, f.p, other, "member")
	mine := f.upload(f.member, f.task, "m.txt", []byte("abc"))
	_, err := f.svc.CompleteAttachment(bg, f.member, mine.ID)
	must(t, err)
	wantStatus(t, f.svc.DeleteAttachment(bg, other, mine.ID), "403")      // not theirs, no attachment.delete
	wantStatus(t, f.svc.DeleteAttachment(bg, f.viewer, mine.ID), "403")   // viewers hold neither permission
	wantStatus(t, f.svc.DeleteAttachment(bg, f.stranger, mine.ID), "404") // unreadable project
	must(t, f.svc.DeleteAttachment(bg, f.owner, mine.ID))                 // attachment.delete (manager)
	if f.mem.Has(mine.StorageKey) || f.count(`SELECT count(*) FROM attachments WHERE id = ?`, mine.ID) != 0 {
		t.Fatal("the object and the row are removed")
	}
	if ev := f.events("attachment.removed"); len(ev) != 1 || ev[0]["attachment_id"] != mine.ID.String() {
		t.Fatalf("attachment.removed: %v", ev)
	}
	wantStatus(t, f.svc.DeleteAttachment(bg, f.owner, mine.ID), "404")
	// The uploader cancels their own pending upload; no removed event for something never announced.
	pending := f.upload(f.member, f.task, "p.txt", []byte("abc"))
	must(t, f.svc.DeleteAttachment(bg, f.member, pending.ID))
	if len(f.events("attachment.removed")) != 1 {
		t.Fatal("pending uploads are never announced")
	}
	// A failing object delete keeps the deleted row for the sweep.
	again := f.upload(f.member, f.task, "q.txt", []byte("abc"))
	f.mem.FailDelete = errBoom
	must(t, f.svc.DeleteAttachment(bg, f.member, again.ID))
	if f.count(`SELECT count(*) FROM attachments WHERE id = ? AND status = 'deleted'`, again.ID) != 1 {
		t.Fatal("row kept for the sweep")
	}
}

func TestSweepAttachments(t *testing.T) {
	f := newCommentFixture(t)
	stalePending := f.upload(f.member, f.task, "old.txt", []byte("abc"))
	freshPending := f.upload(f.member, f.task, "new.txt", []byte("abc"))
	ready := f.upload(f.member, f.task, "ok.txt", []byte("abc"))
	_, err := f.svc.CompleteAttachment(bg, f.member, ready.ID)
	must(t, err)
	f.db.Exec(`UPDATE attachments SET created_at = ? WHERE id = ?`, ago(PendingMaxAge+time.Hour), stalePending.ID)
	// A ready attachment of a purged task is an orphan.
	orphanTask := f.env.task(f.p, nil)
	orphan := f.upload(f.member, orphanTask, "orphan.txt", []byte("abc"))
	_, err = f.svc.CompleteAttachment(bg, f.member, orphan.ID)
	must(t, err)
	f.db.Exec(`DELETE FROM tasks WHERE id = ?`, orphanTask)
	// A deleted row whose object cannot be removed stays until the provider recovers.
	stuck := f.upload(f.member, f.task, "stuck.txt", []byte("abc"))
	f.mem.FailDelete = errBoom
	must(t, f.svc.DeleteAttachment(bg, f.member, stuck.ID))
	res, err := f.svc.SweepAttachments(bg, PendingMaxAge)
	must(t, err)
	if res.Retired != 2 || res.Removed != 0 || f.count(`SELECT count(*) FROM attachments WHERE status = 'deleted'`) != 3 {
		t.Fatalf("with a failing provider: %+v", res)
	}
	f.mem.FailDelete = nil
	res, err = f.svc.SweepAttachments(bg, PendingMaxAge)
	must(t, err)
	if res.Retired != 0 || res.Removed != 3 {
		t.Fatalf("after recovery: %+v", res)
	}
	for _, gone := range []*Attachment{stalePending, orphan, stuck} {
		if f.mem.Has(gone.StorageKey) {
			t.Errorf("object %s survived", gone.Filename)
		}
	}
	if !f.mem.Has(ready.StorageKey) || !f.mem.Has(freshPending.StorageKey) || f.count(`SELECT count(*) FROM attachments`) != 2 {
		t.Fatal("ready and fresh pending attachments are untouched")
	}
}

type brokenStore struct {
	storage.Store
	putErr, getErr error
}

func (b brokenStore) PresignPut(ctx context.Context, key, ct string, size int64, sum string, ttl time.Duration) (storage.PutTarget, error) {
	if b.putErr != nil {
		return storage.PutTarget{}, b.putErr
	}
	return b.Store.PresignPut(ctx, key, ct, size, sum, ttl)
}

func (b brokenStore) PresignGet(ctx context.Context, key, name string, ttl time.Duration) (string, error) {
	if b.getErr != nil {
		return "", b.getErr
	}
	return b.Store.PresignGet(ctx, key, name, ttl)
}

func TestProviderFailures(t *testing.T) {
	f := newCommentFixture(t)
	f.svc.p.Storage = brokenStore{Store: f.mem, putErr: errBoom}
	_, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "a.txt", Size: 1, ContentType: "text/plain"})
	wantCode(t, err, "STORAGE_UNAVAILABLE")
	if f.count(`SELECT count(*) FROM attachments WHERE status = 'pending'`) != 0 {
		t.Fatal("a failed presign must not leave a pending row")
	}
	f.svc.p.Storage = f.mem
	att := f.upload(f.member, f.task, "a.txt", []byte("abc"))
	f.mem.FailHead = errBoom
	_, err = f.svc.CompleteAttachment(bg, f.member, att.ID)
	wantCode(t, err, "STORAGE_UNAVAILABLE")
	f.mem.FailHead = nil
	_, err = f.svc.CompleteAttachment(bg, f.member, att.ID)
	must(t, err)
	f.svc.p.Storage = brokenStore{Store: f.mem, getErr: errBoom}
	_, err = f.svc.DownloadAttachment(bg, f.member, att.ID)
	wantCode(t, err, "STORAGE_UNAVAILABLE")
}

func TestNoStorageConfigured(t *testing.T) {
	f := newCommentFixture(t)
	f.svc.p.Storage = nil
	id := uuid.New()
	_, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "a.txt", Size: 1, ContentType: "text/plain"})
	wantStatus(t, err, "503")
	_, _, err = f.svc.ListAttachments(bg, f.member, f.task, PageRequest{})
	wantCode(t, err, "STORAGE_UNAVAILABLE")
	_, err = f.svc.CompleteAttachment(bg, f.member, id)
	wantCode(t, err, "STORAGE_UNAVAILABLE")
	_, err = f.svc.DownloadAttachment(bg, f.member, id)
	wantCode(t, err, "STORAGE_UNAVAILABLE")
	wantCode(t, f.svc.DeleteAttachment(bg, f.member, id), "STORAGE_UNAVAILABLE")
	if res, err := f.svc.SweepAttachments(bg, PendingMaxAge); err != nil || res != (SweepResult{}) {
		t.Fatalf("the sweep is a no-op without storage: %+v %v", res, err)
	}
}

func TestListAttachmentsPaging(t *testing.T) {
	f := newCommentFixture(t)
	for i := 0; i < 3; i++ {
		a := f.upload(f.member, f.task, "f"+string(rune('a'+i))+".txt", []byte("abc"))
		_, err := f.svc.CompleteAttachment(bg, f.member, a.ID)
		must(t, err)
	}
	p1, next, err := f.svc.ListAttachments(bg, f.viewer, f.task, PageRequest{Limit: 2})
	must(t, err)
	p2, next2, err := f.svc.ListAttachments(bg, f.viewer, f.task, PageRequest{Limit: 2, Cursor: next})
	must(t, err)
	if len(p1) != 2 || len(p2) != 1 || next2 != "" || p1[0].Filename != "fa.txt" || p2[0].Filename != "fc.txt" {
		t.Fatalf("paging: %v %v", p1, p2)
	}
	if _, _, err = f.svc.ListAttachments(bg, f.stranger, f.task, PageRequest{}); err == nil {
		t.Fatal("strangers list nothing")
	}
	if _, _, err = f.svc.ListAttachments(bg, f.viewer, f.task, PageRequest{Cursor: "%%"}); err == nil {
		t.Fatal("bad cursor")
	}
}

func TestRunLoopSweepsAndStops(t *testing.T) {
	f := newCommentFixture(t)
	stale, err := f.svc.StartTimer(bg, f.member, f.task)
	must(t, err)
	f.db.Exec(`UPDATE time_entries SET started_at = ? WHERE id = ?`, ago(30*time.Hour), stale.ID)
	ctx, cancel := context.WithCancel(bg)
	done := make(chan struct{})
	go func() { f.svc.Run(ctx, 20*time.Millisecond, func(error) {}); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !f.entry(stale.ID).AutoStopped {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if !f.entry(stale.ID).AutoStopped {
		t.Fatal("the loop stops stale timers")
	}
	// Failures reach the callback and never stop the loop.
	var seen int
	ctx2, cancel2 := context.WithCancel(bg)
	cancel2()
	f.svc.tick(ctx2, func(error) { seen++ })
	if seen == 0 {
		t.Fatal("expected tick errors with a cancelled context")
	}
	f.svc.tick(ctx2, nil) // a nil callback is allowed
}
