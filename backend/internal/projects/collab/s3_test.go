package collab

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/jaas/jaas/internal/shared/storage"
)

// TestS3AttachmentRoundTrip drives the whole attachment flow against the dev S3 server (docker-compose service s3): init,
// upload with the presigned PUT, complete, presigned download, delete. It skips when the server is unreachable unless
// JAAS_REQUIRE_INTEGRATION=1.
func TestS3AttachmentRoundTrip(t *testing.T) {
	endpoint := os.Getenv("JAAS_TEST_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:9090"
	}
	s3, err := storage.NewS3(bg, storage.S3Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "jaas-files", AccessKey: "test", SecretKey: "test", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s3.EnsureBucket(bg); err != nil {
		if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
			t.Fatalf("object storage required but unavailable: %v", err)
		}
		t.Skipf("object storage unavailable: %v", err)
	}
	f := newCommentFixture(t)
	f.svc.p.Storage = s3
	data := []byte("real object storage bytes")
	tk, err := f.svc.InitAttachment(bg, f.member, f.task, InitInput{Filename: "real.txt", Size: int64(len(data)), ContentType: "text/plain"})
	must(t, err)
	t.Cleanup(func() { _ = s3.Delete(bg, tk.Attachment.StorageKey) })
	_, err = f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	wantCode(t, err, "UPLOAD_MISMATCH") // nothing uploaded yet
	req, _ := http.NewRequest(tk.Upload.Method, tk.Upload.URL, bytes.NewReader(data))
	for k, v := range tk.Upload.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("upload: %v %v", resp, err)
	}
	resp.Body.Close()
	done, err := f.svc.CompleteAttachment(bg, f.member, tk.Attachment.ID)
	must(t, err)
	if done.Status != "ready" {
		t.Fatalf("complete: %+v", done)
	}
	link, err := f.svc.DownloadAttachment(bg, f.viewer, tk.Attachment.ID)
	must(t, err)
	g, err := http.Get(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(g.Body)
	g.Body.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("download body: %q", got)
	}
	must(t, f.svc.DeleteAttachment(bg, f.member, tk.Attachment.ID))
	if _, err := s3.Head(bg, tk.Attachment.StorageKey); err == nil {
		t.Fatal("object must be gone after delete")
	}
}
