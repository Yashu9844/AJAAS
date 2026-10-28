package task

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

const rankAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// slot says where a task goes inside a column: after or before a neighbour, or at the end when both are nil.
type slot struct{ before, after *uuid.UUID }

// colPlace names a column and the task being placed in it (excluded from neighbour lookups).
type colPlace struct{ tenantID, columnID, exclude uuid.UUID }

type bounds struct {
	lo, hi string
	tied   bool // the neighbour shares its rank with another task
}

const liveInColumn = `tenant_id = ? AND column_id = ? AND deleted_at IS NULL AND id <> ?`

// neighbour returns the rank of a live task in the column (the exclusion is the task being placed) and whether another
// live task shares that rank, which makes "next to it" ambiguous.
func neighbour(tx *gorm.DB, p colPlace, id uuid.UUID) (string, bool, error) {
	tenantID, columnID, exclude := p.tenantID, p.columnID, p.exclude
	var r struct {
		Rank string
		Same int
	}
	err := tx.Raw(`SELECT t.rank, (SELECT count(*) FROM tasks o WHERE o.tenant_id = t.tenant_id AND o.column_id = t.column_id
			AND o.deleted_at IS NULL AND o.archived_at IS NULL AND o.id <> t.id AND o.id <> ? AND o.rank = t.rank) AS same
		FROM tasks t WHERE t.id = ? AND t.tenant_id = ? AND t.column_id = ? AND t.deleted_at IS NULL AND t.archived_at IS NULL AND t.id <> ?`,
		exclude, id, tenantID, columnID, exclude).Scan(&r).Error
	if err != nil {
		return "", false, err
	}
	if r.Rank == "" {
		return "", false, kernel.Invalid("before_id", "neighbour is not a task of the target column")
	}
	return r.Rank, r.Same > 0, nil
}

// edge returns the lowest or highest live rank in the column that satisfies cond ("" when there is none).
func edge(tx *gorm.DB, fn string, a edgeArgs) (string, error) {
	var r string
	sql := `SELECT COALESCE(` + fn + `(rank), '') FROM tasks WHERE ` + liveInColumn + ` AND archived_at IS NULL` + a.cond
	err := tx.Raw(sql, append([]any{a.tenantID, a.columnID, a.exclude}, a.args...)...).Scan(&r).Error
	return r, err
}

type edgeArgs struct {
	tenantID, columnID, exclude uuid.UUID
	cond                        string
	args                        []any
}

func (s *Service) neighbours(tx *gorm.DB, p colPlace, at slot) (bounds, error) {
	var b bounds
	var err error
	e := edgeArgs{tenantID: p.tenantID, columnID: p.columnID, exclude: p.exclude}
	switch {
	case at.after != nil:
		if b.lo, b.tied, err = neighbour(tx, p, *at.after); err != nil {
			return b, err
		}
		e.cond, e.args = ` AND rank > ?`, []any{b.lo}
		b.hi, err = edge(tx, "min", e)
	case at.before != nil:
		if b.hi, b.tied, err = neighbour(tx, p, *at.before); err != nil {
			return b, err
		}
		e.cond, e.args = ` AND rank < ?`, []any{b.hi}
		b.lo, err = edge(tx, "max", e)
	default:
		b.lo, err = edge(tx, "max", e)
	}
	return b, err
}

func usable(r string, b bounds) bool {
	return r != "" && len(r) <= maxRankLen && kernel.ValidRank(r) && r > b.lo && (b.hi == "" || r < b.hi)
}

// allocRank returns a rank for the task at the slot. The caller holds the column advisory lock. When the neighbours
// leave no room (equal ranks, or ranks grown too long) the column is rebalanced once and the slot is computed again.
func (s *Service) allocRank(tx *gorm.DB, p colPlace, at slot) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		b, err := s.neighbours(tx, p, at)
		if err != nil {
			return "", err
		}
		if r := kernel.RankBetween(b.lo, b.hi); !b.tied && usable(r, b) {
			return r, nil
		}
		if err := rebalance(tx, p.tenantID, p.columnID); err != nil {
			return "", err
		}
	}
	return "", kernel.Conflict("RANK_UNAVAILABLE", "the column could not be reordered; retry")
}

// rebalance rewrites the ranks of one column with evenly spread values, bounded by rebalanceLimit tasks.
func rebalance(tx *gorm.DB, tenantID, columnID uuid.UUID) error {
	var ids []uuid.UUID
	err := tx.Raw(`SELECT id FROM tasks WHERE tenant_id = ? AND column_id = ? AND deleted_at IS NULL ORDER BY rank, id LIMIT ?`,
		tenantID, columnID, rebalanceLimit+1).Scan(&ids).Error
	if err != nil {
		return err
	}
	if len(ids) > rebalanceLimit {
		return kernel.Conflict("COLUMN_TOO_LARGE", "the column is too large to rebalance in one step")
	}
	ranks := spreadRanks(len(ids))
	rows := make([]map[string]any, len(ids))
	for i, id := range ids {
		rows[i] = map[string]any{"id": id, "rank": ranks[i]}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return tx.Exec(`UPDATE tasks t SET rank = v.rank FROM jsonb_to_recordset(?::jsonb) AS v(id uuid, rank text)
		WHERE t.id = v.id AND t.tenant_id = ?`, string(raw), tenantID).Error
}

// spreadRanks returns n strictly increasing ranks of one fixed width (trailing zeros trimmed), leaving room between
// neighbours for many inserts. kernel.Spread would grow a character every few items on long columns.
func spreadRanks(n int) []string {
	if n == 0 {
		return nil
	}
	width, capacity := 1, uint64(62)
	for capacity < uint64(n+1)*62 {
		width++
		capacity *= 62
	}
	step := capacity / uint64(n+1)
	out := make([]string, n)
	for i := range out {
		out[i] = encodeRank(uint64(i+1)*step, width)
	}
	return out
}

func encodeRank(v uint64, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = rankAlphabet[v%62]
		v /= 62
	}
	return strings.TrimRight(string(buf), "0")
}
