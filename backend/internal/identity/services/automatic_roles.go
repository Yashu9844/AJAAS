package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// AutomaticRoleSource reports, from HR data, who qualifies for the automatic roles: employees (an active employee
// record) and managers (at least one active direct report). M0 keeps no copy of the org chart.
type AutomaticRoleSource interface {
	Members(ctx context.Context, tenantID uuid.UUID) (employees, managers []uuid.UUID, err error)
	MemberOf(ctx context.Context, tenantID, userID uuid.UUID) (employee, manager bool, err error)
}

// AutomaticRoles keeps the automatic employee and manager role assignments in line with HR data.
type AutomaticRoles interface {
	// SetSource installs the HR-backed source; until then syncing is a no-op.
	SetSource(src AutomaticRoleSource)
	SyncTenant(ctx context.Context, tenantID uuid.UUID) (added, removed int, err error)
	SyncUser(ctx context.Context, tenantID, userID uuid.UUID) error
	// Run sweeps every tenant each interval until ctx is cancelled.
	Run(ctx context.Context, interval time.Duration)
}

// AutomaticRolesDeps are the reconciler's collaborators.
type AutomaticRolesDeps struct {
	DB        *gorm.DB
	Transact  Transactor // defaults to DB.Transaction
	Access    repositories.AccessRepository
	UserRoles repositories.UserRoleRepository
	Catalogue Catalogue
	Logger    *zerolog.Logger
}

type automaticRoles struct {
	d   AutomaticRolesDeps
	src AutomaticRoleSource
}

// NewAutomaticRoles builds the reconciler.
func NewAutomaticRoles(d AutomaticRolesDeps) AutomaticRoles {
	if d.Transact == nil {
		d.Transact = dbTransactor(d.DB)
	}
	return &automaticRoles{d: d}
}

func (a *automaticRoles) SetSource(src AutomaticRoleSource) { a.src = src }

// templateRoles returns the tenant's automatic role IDs, provisioning predefined roles if missing.
func (a *automaticRoles) templateRoles(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) (employee, manager uuid.UUID, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		emp, err := a.d.Access.RoleByTemplateKey(ctx, tx, tenantID, access.TemplateEmployee)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		mgr, err := a.d.Access.RoleByTemplateKey(ctx, tx, tenantID, access.TemplateManager)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		if emp != nil && mgr != nil {
			return emp.ID, mgr.ID, nil
		}
		if err := a.d.Catalogue.ProvisionTenant(ctx, tx, tenantID); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}
	return uuid.Nil, uuid.Nil, errAutomaticRole
}

func (a *automaticRoles) SyncTenant(ctx context.Context, tenantID uuid.UUID) (added, removed int, err error) {
	if a.src == nil {
		return 0, 0, nil
	}
	employees, managers, err := a.src.Members(ctx, tenantID)
	if err != nil {
		return 0, 0, err
	}
	err = a.d.Transact(ctx, func(tx *gorm.DB) error {
		emp, mgr, err := a.templateRoles(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		want := map[[2]uuid.UUID]bool{}
		for _, u := range employees {
			want[[2]uuid.UUID{u, emp}] = true
		}
		for _, u := range managers {
			want[[2]uuid.UUID{u, mgr}] = true
		}
		current, err := a.d.Access.AutomaticAssignments(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		added, removed, err = a.reconcile(ctx, tx, tenantID, want, current, map[uuid.UUID]bool{emp: true, mgr: true})
		return err
	})
	return added, removed, err
}

// reconcile adds wanted automatic assignments and removes stale ones among the automatic roles.
func (a *automaticRoles) reconcile(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, want map[[2]uuid.UUID]bool, current []models.UserRole, autoRoles map[uuid.UUID]bool) (int, int, error) {
	added, removed := 0, 0
	have := map[[2]uuid.UUID]bool{}
	for _, ur := range current {
		key := [2]uuid.UUID{ur.UserID, ur.RoleID}
		have[key] = true
		if !want[key] && autoRoles[ur.RoleID] {
			if err := a.d.UserRoles.Delete(ctx, tx, tenantID, ur.UserID, ur.RoleID); err != nil {
				return added, removed, err
			}
			removed++
		}
	}
	for key := range want {
		if have[key] {
			continue
		}
		ur := &models.UserRole{UserID: key[0], RoleID: key[1], TenantID: tenantID, Source: string(access.SourceAutomatic)}
		if err := a.d.UserRoles.Create(ctx, tx, ur); err != nil {
			return added, removed, err
		}
		added++
	}
	return added, removed, nil
}

func (a *automaticRoles) SyncUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	if a.src == nil {
		return nil
	}
	isEmployee, isManager, err := a.src.MemberOf(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	return a.d.Transact(ctx, func(tx *gorm.DB) error {
		emp, mgr, err := a.templateRoles(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		want := map[[2]uuid.UUID]bool{}
		if isEmployee {
			want[[2]uuid.UUID{userID, emp}] = true
		}
		if isManager {
			want[[2]uuid.UUID{userID, mgr}] = true
		}
		urs, err := a.d.UserRoles.FindByUserID(ctx, tx, tenantID, userID)
		if err != nil {
			return err
		}
		var current []models.UserRole
		for _, ur := range urs {
			if ur.Source == string(access.SourceAutomatic) {
				current = append(current, ur)
			}
		}
		_, _, err = a.reconcile(ctx, tx, tenantID, want, current, map[uuid.UUID]bool{emp: true, mgr: true})
		return err
	})
}

func (a *automaticRoles) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		a.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *automaticRoles) sweep(ctx context.Context) {
	if a.src == nil {
		return
	}
	ids, err := a.d.Access.TenantIDs(ctx, a.d.DB, true)
	if err != nil {
		a.warn(err, uuid.Nil)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if _, _, err := a.SyncTenant(ctx, id); err != nil {
			a.warn(err, id)
		}
	}
}

func (a *automaticRoles) warn(err error, tenantID uuid.UUID) {
	if a.d.Logger != nil {
		a.d.Logger.Warn().Err(err).Str("tenant_id", tenantID.String()).Msg("automatic role sync failed")
	}
}
