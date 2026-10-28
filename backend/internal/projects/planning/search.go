package planning

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var ticketPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-([0-9]{1,9})$`)

// SearchParams is a search request; ProjectID narrows it to one readable project.
type SearchParams struct {
	Query     string
	ProjectID *uuid.UUID
	Limit     int
}

// SearchHit is a card with how it matched ("ticket" or "text") and its rank.
type SearchHit struct {
	Card
	Match string  `json:"match"`
	Score float64 `json:"score"`
}

// ClampSearchLimit applies the search default (20) and maximum (50).
func ClampSearchLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 20
	}
	return min(n, maxSearchLimit)
}

// Search finds tasks by ticket id (KEY-12) and by full text over title and description, only in projects the caller
// can read, always inside the caller tenant (TK-14, PG9). Text matches are ordered by ts_rank.
func (s *Service) Search(ctx context.Context, a api.Actor, p SearchParams) ([]SearchHit, error) {
	q := strings.TrimSpace(p.Query)
	if q == "" || len([]rune(q)) > maxSearchQuery {
		return nil, kernel.Invalid("q", "length must be between 1 and 200")
	}
	ids, err := s.searchProjects(ctx, a, p.ProjectID)
	if err != nil || len(ids) == 0 {
		return []SearchHit{}, err
	}
	hits, err := s.ticketHit(ctx, a, ids, q)
	if err != nil {
		return nil, err
	}
	if room := p.Limit - len(hits); room > 0 {
		byText, err := s.textHits(ctx, a, ids, q, room)
		if err != nil {
			return nil, err
		}
		for _, h := range byText {
			if len(hits) == 0 || h.ID != hits[0].ID {
				hits = append(hits, h)
			}
		}
	}
	if len(hits) > p.Limit {
		hits = hits[:p.Limit]
	}
	if hits == nil {
		hits = []SearchHit{}
	}
	return hits, nil
}

// searchProjects is the readable project set, narrowed to one project when asked (404 when it is not readable).
func (s *Service) searchProjects(ctx context.Context, a api.Actor, only *uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.taskProjects(ctx, a)
	if err != nil || only == nil {
		return ids, err
	}
	for _, id := range ids {
		if id == *only {
			return []uuid.UUID{id}, nil
		}
	}
	return nil, kernel.NotFound("project")
}

// ticketHit resolves a query shaped like KEY-12 to that task when its project is readable.
func (s *Service) ticketHit(ctx context.Context, a api.Actor, ids []uuid.UUID, q string) ([]SearchHit, error) {
	m := ticketPattern.FindStringSubmatch(q)
	if m == nil {
		return nil, nil
	}
	number, _ := strconv.Atoi(m[2])
	where := `t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND p.key = ? AND t.number = ? AND ` + liveTask
	rows, err := cardQuery{where: where, args: []any{a.TenantID, idArray(ids), strings.ToUpper(m[1]), number}, order: `t.id`, limit: 1}.run(ctx, s.db)
	hits := make([]SearchHit, 0, len(rows))
	for _, c := range rows {
		hits = append(hits, SearchHit{Card: c, Match: "ticket", Score: 1})
	}
	return hits, err
}

type scoredID struct {
	ID    uuid.UUID
	Score float64
}

// textHits ranks matches with the GIN-indexed tsvector, then loads the cards of the winners.
func (s *Service) textHits(ctx context.Context, a api.Actor, ids []uuid.UUID, q string, limit int) ([]SearchHit, error) {
	var scored []scoredID
	err := s.db.WithContext(ctx).Raw(`SELECT t.id, ts_rank(t.search, websearch_to_tsquery('simple', ?)) AS score FROM tasks t
		WHERE t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND `+liveTask+` AND t.search @@ websearch_to_tsquery('simple', ?)
		ORDER BY score DESC, t.id LIMIT ?`, q, a.TenantID, idArray(ids), q, limit).Scan(&scored).Error
	if err != nil || len(scored) == 0 {
		return nil, err
	}
	found := make([]uuid.UUID, len(scored))
	for i, r := range scored {
		found[i] = r.ID
	}
	cards, err := cardQuery{where: `t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND t.id = ANY(?::uuid[])`, args: []any{a.TenantID, idArray(ids), idArray(found)}, order: `t.id`, limit: limit}.run(ctx, s.db)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]Card, len(cards))
	for _, c := range cards {
		byID[c.ID] = c
	}
	hits := make([]SearchHit, 0, len(scored))
	for _, r := range scored {
		if c, ok := byID[r.ID]; ok {
			hits = append(hits, SearchHit{Card: c, Match: "text", Score: r.Score})
		}
	}
	return hits, nil
}
