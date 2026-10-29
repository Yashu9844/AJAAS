package webhooks

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"gorm.io/gorm"
)

// injector fails the n-th SQL statement issued while it is armed (n counts from 1; 0 only counts).
type injector struct {
	armed bool
	count int
	n     int
}

func (in *injector) hook(tx *gorm.DB) {
	if !in.armed {
		return
	}
	in.count++
	if in.n > 0 && in.count == in.n {
		_ = tx.AddError(errors.New("injected database failure"))
	}
}

func (e *env) injector() *injector {
	in := &injector{}
	cb := e.db.Callback()
	_ = cb.Create().Before("gorm:create").Register("inj:create", in.hook)
	_ = cb.Query().Before("gorm:query").Register("inj:query", in.hook)
	_ = cb.Update().Before("gorm:update").Register("inj:update", in.hook)
	_ = cb.Delete().Before("gorm:delete").Register("inj:delete", in.hook)
	_ = cb.Row().Before("gorm:row").Register("inj:row", in.hook)
	_ = cb.Raw().Before("gorm:raw").Register("inj:raw", in.hook)
	return in
}

// Every statement of the main flows is failed in turn: the service must return an error and never swallow it (DPJ-24).
func TestEveryStatementFailureIsReported(t *testing.T) {
	e := newEnv(t)
	inj := e.injector()
	owner := e.user(false)
	seq := 0
	type scenario struct {
		name string
		run  func() func() error
	}
	scenarios := []scenario{
		{"webhook management", func() func() error {
			seq++
			p := e.project(owner, "FW"+strconv.Itoa(seq))
			return func() error {
				w, err := e.svc.CreateWebhook(bg, owner, p.ID, WebhookInput{Name: "w", URL: "https://example.com/x"})
				if err != nil {
					return err
				}
				if _, err = e.svc.Webhooks(bg, owner, p.ID); err != nil {
					return err
				}
				if _, err = e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{Name: ptr("w2")}); err != nil {
					return err
				}
				if _, err = e.svc.RotateWebhookSecret(bg, owner, p.ID, w.ID); err != nil {
					return err
				}
				return e.svc.DeleteWebhook(bg, owner, p.ID, w.ID)
			}
		}},
		{"deliveries", func() func() error {
			seq++
			p := e.project(owner, "FD"+strconv.Itoa(seq))
			inj.armed = false
			w := e.webhook(owner, p, WebhookInput{Format: "slack"})
			return func() error {
				d, err := e.svc.TestWebhook(bg, owner, p.ID, w.ID)
				if err != nil {
					return err
				}
				if _, _, err = e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 5, Status: StatusDelivered}); err != nil {
					return err
				}
				_, err = e.svc.Redeliver(bg, owner, p.ID, w.ID, d.ID)
				return err
			}
		}},
		{"dispatch and send", func() func() error {
			seq++
			p := e.project(owner, "FS"+strconv.Itoa(seq))
			inj.armed = false
			e.db.Exec(`DELETE FROM webhook_deliveries`) // one delivery only: concurrent sends would make the statement count racy
			e.webhook(owner, p, WebhookInput{Format: "discord"})
			tid := e.task(owner, p, 1)
			e.doer.def = Result{Err: errConnect}
			e.taskEvent("task.created", p.ID, tid)
			return func() error {
				if _, err := e.svc.DispatchOnce(bg); err != nil {
					return err
				}
				_, err := e.svc.SendOnce(bg)
				return err
			}
		}},
		{"integrations", func() func() error {
			seq++
			p := e.project(owner, "FI"+strconv.Itoa(seq))
			return func() error {
				it, err := e.svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: "github", Name: "r"})
				if err != nil {
					return err
				}
				if _, err = e.svc.Integrations(bg, owner, p.ID); err != nil {
					return err
				}
				if _, err = e.svc.RotateIntegrationSecret(bg, owner, p.ID, it.ID); err != nil {
					return err
				}
				return e.svc.DeleteIntegration(bg, owner, p.ID, it.ID)
			}
		}},
		{"incoming and links", func() func() error {
			seq++
			key := "FL" + strconv.Itoa(seq)
			p := e.project(owner, key)
			inj.armed = false
			tid := e.task(owner, p, 1)
			it, err := e.svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: "gitea", Name: "r"})
			if err != nil {
				t.Fatal(err)
			}
			body := []byte(fmt.Sprintf(`{"action":"closed","pull_request":{"number":5,"html_url":"https://git.example.com/o/r/pulls/5","title":"%s-1","merged":true,"head":{"ref":"x"}}}`, key))
			return func() error {
				in := Incoming{Provider: ProviderGitea, ID: it.ID.String(), Event: "pull_request", Signature: hmacHex([]byte(it.Secret), body), Body: body}
				if _, err := e.svc.Receive(bg, in); err != nil {
					return err
				}
				_, err := e.svc.TaskLinks(bg, owner, tid)
				return err
			}
		}},
	}
	for _, sc := range scenarios {
		prepare := func() func() error {
			inj.armed = false
			e.doer.def = Result{Status: 200}
			run := sc.run()
			inj.armed, inj.count, inj.n = true, 0, 0
			return run
		}
		if err := prepare()(); err != nil {
			t.Fatalf("%s: dry run failed: %v", sc.name, err)
		}
		total := inj.count
		for n := 1; n <= total; n++ {
			run := prepare()
			inj.n = n
			if err := run(); err == nil {
				t.Errorf("%s: statement %d of %d failed silently", sc.name, n, total)
			}
			inj.armed = false
		}
	}
	inj.armed = false
}
