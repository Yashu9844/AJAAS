package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jaas/jaas/pkg/pmclient"
)

type printer struct {
	out  *os.File
	json bool
}

func (p printer) value(v any) {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func str(m pmclient.Map, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	if v, ok := m[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// rows prints a table of the compact task fields.
func (p printer) tasks(items []pmclient.Map) {
	if p.json {
		p.value(items)
		return
	}
	for _, t := range items {
		fmt.Fprintf(p.out, "%-10s %-8s %-10s %s\n", str(t, "key"), str(t, "priority"), str(t, "column_category"), str(t, "title"))
	}
	fmt.Fprintf(p.out, "(%d tasks)\n", len(items))
}

func dispatch(ctx context.Context, c *pmclient.Client, argv []string, p printer) error {
	cmd, rest := argv[0], argv[1:]
	switch cmd {
	case "projects":
		return projects(ctx, c, rest, p)
	case "tasks":
		return tasks(ctx, c, rest, p)
	case "task":
		return one(ctx, c, rest, p)
	case "create":
		return create(ctx, c, rest, p)
	case "move":
		return move(ctx, c, rest, p)
	case "comment":
		return comment(ctx, c, rest, p)
	case "time":
		return logTime(ctx, c, rest, p)
	case "mywork":
		w, err := c.MyWork(ctx)
		p.value(w)
		return err
	case "search":
		return search(ctx, c, rest, p)
	default:
		return fmt.Errorf("unknown command %q; run `jaas-pm help`", cmd)
	}
}

func need(rest []string, n int, what string) error {
	if len(rest) < n {
		return errors.New("usage: " + what)
	}
	return nil
}

func projects(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	q := fs.String("query", "", "filter by name or key")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	page, err := c.Projects(ctx, "", *q, "", 50)
	if err != nil {
		return err
	}
	if p.json {
		p.value(page.Items)
		return nil
	}
	for _, m := range page.Items {
		fmt.Fprintf(p.out, "%-10s %-10s %s\n", str(m, "key"), str(m, "my_role"), str(m, "name"))
	}
	return nil
}

func tasks(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 1, "jaas-pm tasks PROJECT [--column NAME] [--limit N]"); err != nil {
		return err
	}
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	col := fs.String("column", "", "column name")
	limit := fs.Int("limit", 50, "max tasks")
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	pid, err := projectID(ctx, c, rest[0])
	if err != nil {
		return err
	}
	q := url.Values{"limit": {strconv.Itoa(*limit)}}
	if *col != "" {
		id, cerr := columnID(ctx, c, pid, *col)
		if cerr != nil {
			return cerr
		}
		q.Set("column_id", id)
	}
	page, err := c.Tasks(ctx, pid, q)
	if err != nil {
		return err
	}
	p.tasks(page.Items)
	return nil
}

func projectID(ctx context.Context, c *pmclient.Client, ref string) (string, error) {
	if len(ref) == 36 && strings.Count(ref, "-") == 4 {
		return ref, nil
	}
	page, err := c.Projects(ctx, "", ref, "", 50)
	if err != nil {
		return "", err
	}
	for _, m := range page.Items {
		if strings.EqualFold(str(m, "key"), ref) {
			return str(m, "id"), nil
		}
	}
	return "", fmt.Errorf("no project with key %s", ref)
}

func columnID(ctx context.Context, c *pmclient.Client, pid, ref string) (string, error) {
	cols, err := c.Columns(ctx, pid)
	if err != nil {
		return "", err
	}
	for _, m := range cols {
		if strings.EqualFold(str(m, "name"), ref) || strings.EqualFold(str(m, "key"), ref) {
			return str(m, "id"), nil
		}
	}
	return "", fmt.Errorf("no column named %q", ref)
}

func one(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 1, "jaas-pm task KEY-12"); err != nil {
		return err
	}
	t, err := c.Task(ctx, rest[0])
	if err != nil {
		return err
	}
	p.value(pmclient.CompactTask(t, true))
	return nil
}

func create(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 2, `jaas-pm create PROJECT "Title" [--priority P] [--description TEXT]`); err != nil {
		return err
	}
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	prio := fs.String("priority", "", "none|low|medium|high|urgent")
	desc := fs.String("description", "", "markdown")
	if err := fs.Parse(rest[2:]); err != nil {
		return err
	}
	pid, err := projectID(ctx, c, rest[0])
	if err != nil {
		return err
	}
	in := pmclient.Map{"title": rest[1]}
	if *prio != "" {
		in["priority"] = *prio
	}
	if *desc != "" {
		in["description"] = *desc
	}
	t, err := c.CreateTask(ctx, pid, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(p.out, "created %s\n", str(t, "key"))
	return nil
}

func move(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 2, `jaas-pm move KEY-12 "Column name"`); err != nil {
		return err
	}
	t, err := c.Task(ctx, rest[0])
	if err != nil {
		return err
	}
	col, err := columnID(ctx, c, str(t, "project_id"), rest[1])
	if err != nil {
		return err
	}
	if _, err = c.MoveTask(ctx, rest[0], col); err != nil {
		return err
	}
	fmt.Fprintf(p.out, "moved %s to %s\n", rest[0], rest[1])
	return nil
}

func comment(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 2, `jaas-pm comment KEY-12 "Text"`); err != nil {
		return err
	}
	if _, err := c.Comment(ctx, rest[0], strings.Join(rest[1:], " ")); err != nil {
		return err
	}
	fmt.Fprintln(p.out, "comment added")
	return nil
}

func logTime(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 2, "jaas-pm time KEY-12 MINUTES [--note TEXT]"); err != nil {
		return err
	}
	fs := flag.NewFlagSet("time", flag.ContinueOnError)
	note := fs.String("note", "", "short note")
	if err := fs.Parse(rest[2:]); err != nil {
		return err
	}
	mins, err := strconv.Atoi(rest[1])
	if err != nil || mins <= 0 {
		return errors.New("MINUTES must be a positive number")
	}
	if _, err = c.LogTime(ctx, rest[0], mins, *note); err != nil {
		return err
	}
	fmt.Fprintf(p.out, "logged %d minutes on %s\n", mins, rest[0])
	return nil
}

func search(ctx context.Context, c *pmclient.Client, rest []string, p printer) error {
	if err := need(rest, 1, "jaas-pm search TEXT"); err != nil {
		return err
	}
	items, err := c.Search(ctx, strings.Join(rest, " "), 25)
	if err != nil {
		return err
	}
	p.tasks(items)
	return nil
}
