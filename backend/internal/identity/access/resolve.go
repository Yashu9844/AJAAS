package access

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

// MaxRoleDepth caps role inheritance chains.
const MaxRoleDepth = 10

// Resolution errors.
var (
	ErrRoleCycle   = errors.New("role inheritance must not form a cycle")
	ErrRoleTooDeep = errors.New("role inheritance is deeper than 10 levels")
)

// AssignmentSource says how a user came to hold a role.
type AssignmentSource string

// Assignment sources stored on user_roles.source.
const (
	SourceManual    AssignmentSource = "manual"
	SourceAutomatic AssignmentSource = "automatic"
)

// SourceKind explains where an effective permission comes from.
type SourceKind string

// Source kinds reported by the explain-access API.
const (
	SourceRole          SourceKind = "role"
	SourceAutomaticRole SourceKind = "automatic_role"
	SourceInherited     SourceKind = "inherited"
	SourceUserGrant     SourceKind = "user_grant"
)

// RoleNode is one tenant role with its own grants and optional parent.
type RoleNode struct {
	ID        uuid.UUID
	Name      string
	ParentID  *uuid.UUID
	Automatic bool
	Grants    []Grant
}

// Assignment is a role held by the user.
type Assignment struct {
	RoleID uuid.UUID
	Source AssignmentSource
}

// UserGrant is a grant given to one user, optionally until ExpiresAt.
type UserGrant struct {
	Grant
	ExpiresAt *time.Time
}

// Input is everything needed to resolve one user's effective access.
type Input struct {
	Now        time.Time
	Roles      []RoleNode // every role of the tenant (or at least every assigned role and its ancestors)
	Assigned   []Assignment
	UserGrants []UserGrant
	Blocks     []string // permission keys
}

// Source is one reason a permission is held.
type Source struct {
	Kind     SourceKind `json:"kind"`
	RoleID   string     `json:"role_id,omitempty"`
	RoleName string     `json:"role_name,omitempty"`
	Scope    Scope      `json:"scope"`
}

// Entry is one effective permission: the widest scope held, why, and whether a block removes it.
type Entry struct {
	Permission string   `json:"permission"`
	Scope      Scope    `json:"scope"`
	Sources    []Source `json:"sources"`
	Blocked    bool     `json:"blocked"`
}

// Effective maps permission keys to entries.
type Effective map[string]*Entry

// ScopeFor returns the widest unblocked scope for a permission key (ScopeNone when not held).
func (e Effective) ScopeFor(permission string) Scope {
	if en, ok := e[permission]; ok && !en.Blocked {
		return en.Scope
	}
	return ScopeNone
}

// Missing returns the wanted grants that e does not cover.
func (e Effective) Missing(want []Grant) []Grant {
	var out []Grant
	for _, g := range want {
		if !Covers(e.ScopeFor(g.Permission), g.Scope) {
			out = append(out, g)
		}
	}
	return out
}

// Permissions lists the held (unblocked) permission keys, sorted.
func (e Effective) Permissions() []string {
	out := make([]string, 0, len(e))
	for k, en := range e {
		if !en.Blocked && en.Scope.Valid() {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (e Effective) add(permission string, src Source) {
	en, ok := e[permission]
	if !ok {
		en = &Entry{Permission: permission}
		e[permission] = en
	}
	en.Scope = Wider(en.Scope, src.Scope)
	en.Sources = append(en.Sources, src)
}

// Resolve computes effective access: assigned roles and their ancestors, unexpired user grants, minus blocks.
func Resolve(in Input) (Effective, error) {
	byID := make(map[uuid.UUID]*RoleNode, len(in.Roles))
	for i := range in.Roles {
		byID[in.Roles[i].ID] = &in.Roles[i]
	}
	eff := Effective{}
	for _, a := range in.Assigned {
		chain, err := ancestry(byID, a.RoleID)
		if err != nil {
			return nil, err
		}
		for depth, r := range chain {
			kind := SourceRole
			if depth > 0 {
				kind = SourceInherited
			} else if a.Source == SourceAutomatic || r.Automatic {
				kind = SourceAutomaticRole
			}
			for _, g := range r.Grants {
				eff.add(g.Permission, Source{Kind: kind, RoleID: r.ID.String(), RoleName: r.Name, Scope: g.Scope})
			}
		}
	}
	for _, g := range in.UserGrants {
		if g.ExpiresAt == nil || g.ExpiresAt.After(in.Now) {
			eff.add(g.Permission, Source{Kind: SourceUserGrant, Scope: g.Scope})
		}
	}
	for _, b := range in.Blocks {
		if _, ok := eff[b]; !ok {
			eff[b] = &Entry{Permission: b}
		}
		eff[b].Blocked = true
	}
	return eff, nil
}

// ancestry returns the role followed by its ancestors; unknown roles yield an empty chain.
func ancestry(byID map[uuid.UUID]*RoleNode, id uuid.UUID) ([]*RoleNode, error) {
	var chain []*RoleNode
	seen := map[uuid.UUID]bool{}
	for cur, ok := byID[id]; ok; cur, ok = next(byID, cur) {
		if seen[cur.ID] {
			return nil, ErrRoleCycle
		}
		if len(chain) == MaxRoleDepth {
			return nil, ErrRoleTooDeep
		}
		seen[cur.ID] = true
		chain = append(chain, cur)
	}
	return chain, nil
}

func next(byID map[uuid.UUID]*RoleNode, r *RoleNode) (*RoleNode, bool) {
	if r.ParentID == nil {
		return nil, false
	}
	p, ok := byID[*r.ParentID]
	return p, ok
}

// CheckParent validates making parent the parent of role: no cycle, chain within MaxRoleDepth.
func CheckParent(roles []RoleNode, role, parent uuid.UUID) error {
	if role == parent {
		return ErrRoleCycle
	}
	byID := make(map[uuid.UUID]*RoleNode, len(roles))
	for i := range roles {
		r := roles[i]
		byID[r.ID] = &r
	}
	if r, ok := byID[role]; ok {
		p := parent
		r.ParentID = &p
	} else {
		p := parent
		byID[role] = &RoleNode{ID: role, ParentID: &p}
	}
	_, err := ancestry(byID, role)
	return err
}
