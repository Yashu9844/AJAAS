package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService manages tenant user profiles and statuses.
type UserService interface {
	CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)
	InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)
	GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)
	ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error)
	UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error)
	DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error
	ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)
}

// UserOption customises a UserService.
type UserOption func(*userService)

// WithRolePermissions enables the no-privilege-escalation check when users are created with roles.
func WithRolePermissions(rp repositories.RolePermissionRepository) UserOption {
	return func(s *userService) { s.rolePermRepo = rp }
}

type userService struct {
	rolePermRepo repositories.RolePermissionRepository
	userRepo     repositories.UserRepository
	roleRepo     repositories.RoleRepository
	userRoleRepo repositories.UserRoleRepository
	sessionRepo  repositories.SessionRepository
	tokenRepo    repositories.RefreshTokenRepository
	publisher    queue.EventPublisher
	auditSvc     AuditService
}

// UserDeactivationConverger converges dependent state when a user is deactivated (Module 1 org mappings,
// Module 2 employee profile). Registered at boot by internal/app after the modules are wired.
type UserDeactivationConverger interface {
	DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error
}

var (
	convergersMu sync.RWMutex
	convergers   []UserDeactivationConverger
)

// AddUserDeactivationConverger registers a cross-module convergence hook that runs inside the deactivation
// transaction (so by the time the API answers 200, dependent state has converged).
func AddUserDeactivationConverger(c UserDeactivationConverger) {
	if c == nil {
		return
	}
	convergersMu.Lock()
	convergers = append(convergers, c)
	convergersMu.Unlock()
}

// SetUserDeactivationConverger replaces all registered hooks with c (nil clears them).
func SetUserDeactivationConverger(c UserDeactivationConverger) {
	convergersMu.Lock()
	convergers = nil
	convergersMu.Unlock()
	AddUserDeactivationConverger(c)
}

// NewUserService creates a new UserService.
func NewUserService(
	userRepo repositories.UserRepository,
	roleRepo repositories.RoleRepository,
	userRoleRepo repositories.UserRoleRepository,
	sessionRepo repositories.SessionRepository,
	tokenRepo repositories.RefreshTokenRepository,
	publisher queue.EventPublisher,
	auditSvc AuditService,
	opts ...UserOption,
) UserService {
	s := &userService{
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		userRoleRepo: userRoleRepo,
		sessionRepo:  sessionRepo,
		tokenRepo:    tokenRepo,
		publisher:    publisher,
		auditSvc:     auditSvc,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// resolveRoles validates the requested role ids (de-duplicated, must exist in the tenant) and enforces that the caller
// may assign them, BEFORE anything is written.
func (s *userService) resolveRoles(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, raw []string) ([]*models.Role, error) {
	var roles []*models.Role
	seen := map[uuid.UUID]bool{}
	for _, rIDStr := range raw {
		roleID, err := uuid.Parse(rIDStr)
		if err != nil {
			return nil, sharedErrors.ErrValidation
		}
		if seen[roleID] {
			continue
		}
		seen[roleID] = true
		role, err := s.roleRepo.FindByID(ctx, tx, tenantID, roleID)
		if err != nil {
			return nil, err
		}
		if role == nil {
			return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: fmt.Sprintf("Role ID %s not found", rIDStr), StatusCode: 404}
		}
		roles = append(roles, role)
	}
	if len(roles) > 0 {
		if s.rolePermRepo == nil {
			if caller, ok := CallerFrom(ctx); ok && !caller.IsTenantAdmin() {
				return nil, forbiddenGrant("Role assignment is not permitted for this caller")
			}
		} else if err := ensureCanAssignRoles(ctx, tx, tenantID, roles, s.userRoleRepo, s.rolePermRepo); err != nil {
			return nil, err
		}
	}
	return roles, nil
}

func (s *userService) CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error) {
	roles, err := s.resolveRoles(ctx, tx, tenantID, req.RoleIDs)
	if err != nil {
		return nil, err
	}

	// Verify email uniqueness
	existing, err := s.userRepo.FindByEmail(ctx, tx, tenantID, req.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}

	// Bcrypt hash password with cost 12
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(hashed),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Status:       "active",
	}
	user.TenantID = tenantID

	if req.Phone != "" {
		user.Phone = &req.Phone
	}

	if err := s.userRepo.Create(ctx, tx, user); err != nil {
		return nil, err
	}

	// Associate the (already validated) roles
	var assignedRoleIDs []uuid.UUID
	for _, role := range roles {
		ur := &models.UserRole{UserID: user.ID, RoleID: role.ID, TenantID: tenantID}
		if err := s.userRoleRepo.Create(ctx, tx, ur); err != nil {
			return nil, err
		}
		assignedRoleIDs = append(assignedRoleIDs, role.ID)
	}

	res := mapUserToResponse(user)
	if err := s.attachRoles(ctx, tx, tenantID, res); err != nil {
		return nil, err
	}

	// Publish Event & Audit Log
	eventPayload := events.UserCreatedPayload{
		UserID:    user.ID,
		TenantID:  tenantID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Status:    user.Status,
		RoleIDs:   assignedRoleIDs,
		CreatedAt: user.CreatedAt,
	}
	evt := events.NewEvent(events.TypeUserCreated, events.RoutingKeyUserCreated, tenantID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyUserCreated, evt)

	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "user.created", "user", user.ID.String(), eventPayload, "", "")

	return res, nil
}

func (s *userService) InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error) {
	// Verify email uniqueness
	existing, err := s.userRepo.FindByEmail(ctx, tx, tenantID, req.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}

	// Placeholder dummy password for invited status user
	dummyPassword, _ := uuid.NewRandom()
	hashed, _ := bcrypt.GenerateFromPassword([]byte(dummyPassword.String()), 12)

	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(hashed),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Status:       "invited",
	}
	user.TenantID = tenantID

	if err := s.userRepo.Create(ctx, tx, user); err != nil {
		return nil, err
	}

	// Map and associate default roles
	var assignedRoleIDs []uuid.UUID
	if len(req.RoleIDs) > 0 {
		seenRoles := map[uuid.UUID]bool{}
		for _, rIDStr := range req.RoleIDs {
			roleID, err := uuid.Parse(rIDStr)
			if err != nil {
				return nil, sharedErrors.ErrValidation
			}
			if seenRoles[roleID] {
				continue
			}
			seenRoles[roleID] = true

			role, err := s.roleRepo.FindByID(ctx, tx, tenantID, roleID)
			if err != nil {
				return nil, err
			}
			if role == nil {
				return nil, &sharedErrors.AppError{
					Code:       "NOT_FOUND",
					Message:    fmt.Sprintf("Role ID %s not found", rIDStr),
					StatusCode: 404,
				}
			}

			ur := &models.UserRole{
				UserID:   user.ID,
				RoleID:   roleID,
				TenantID: tenantID,
			}
			if err := s.userRoleRepo.Create(ctx, tx, ur); err != nil {
				return nil, err
			}
			assignedRoleIDs = append(assignedRoleIDs, roleID)
		}
	}

	res := mapUserToResponse(user)

	// Publish Event & Audit Log
	eventPayload := events.UserInvitedPayload{
		UserID:    user.ID,
		TenantID:  tenantID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		InvitedAt: time.Now(),
	}
	evt := events.NewEvent(events.TypeUserInvited, events.RoutingKeyUserInvited, tenantID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyUserInvited, evt)

	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "user.invited", "user", user.ID.String(), eventPayload, "", "")

	return res, nil
}

func (s *userService) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, sharedErrors.ErrNotFound
	}
	res := mapUserToResponse(user)
	if err := s.attachRoles(ctx, db, tenantID, res); err != nil {
		return nil, err
	}
	return res, nil
}

// attachRoles fills Roles on the given responses with one batched query.
func (s *userService) attachRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, users ...*dto.UserResponse) error {
	ids := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		id, err := uuid.Parse(u.ID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	urs, err := s.userRoleRepo.FindByUserIDs(ctx, db, tenantID, ids)
	if err != nil {
		return err
	}
	byUser := make(map[string][]dto.RoleSummary, len(users))
	for _, ur := range urs {
		if ur.Role != nil {
			byUser[ur.UserID.String()] = append(byUser[ur.UserID.String()], dto.RoleSummary{ID: ur.Role.ID.String(), Name: ur.Role.Name})
		}
	}
	for _, u := range users {
		u.Roles = byUser[u.ID]
		if u.Roles == nil {
			u.Roles = []dto.RoleSummary{}
		}
	}
	return nil
}

func (s *userService) ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error) {
	users, total, err := s.userRepo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
		return nil, err
	}

	data := make([]dto.UserResponse, len(users))
	ptrs := make([]*dto.UserResponse, len(users))
	for i := range users {
		data[i] = *mapUserToResponse(&users[i])
		ptrs[i] = &data[i]
	}
	if err := s.attachRoles(ctx, db, tenantID, ptrs...); err != nil {
		return nil, err
	}

	totalPages := int(total / int64(perPage))
	if total%int64(perPage) != 0 {
		totalPages++
	}

	return &dto.UserListResponse{
		Data: data,
		Meta: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *userService) UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Phone != nil {
		user.Phone = req.Phone
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := s.userRepo.Update(ctx, tx, user); err != nil {
		return nil, err
	}

	res := mapUserToResponse(user)
	if err := s.attachRoles(ctx, tx, tenantID, res); err != nil {
		return nil, err
	}
	return res, nil
}

// ActivateUser re-enables a deactivated user (previously revoked sessions stay revoked; the user must log in again).
func (s *userService) ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if user.Status == "active" {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "User is already active", StatusCode: 409}
	}
	user.Status = "active"
	if err := s.userRepo.Update(ctx, tx, user); err != nil {
		return nil, err
	}
	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "user.activated", "user", id.String(), nil, "", "")
	res := mapUserToResponse(user)
	if err := s.attachRoles(ctx, tx, tenantID, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *userService) DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if user == nil {
		return sharedErrors.ErrNotFound
	}

	if user.Status == "inactive" {
		return &sharedErrors.AppError{
			Code:       "CONFLICT",
			Message:    "User is already deactivated",
			StatusCode: 409,
		}
	}

	user.Status = "inactive"
	if err := s.userRepo.Update(ctx, tx, user); err != nil {
		return err
	}

	// AU-010: Revoke active sessions and refresh tokens on deactivation
	if err := s.sessionRepo.RevokeAllByUserID(ctx, tx, tenantID, id); err != nil {
		return err
	}
	if err := s.tokenRepo.RevokeAllByUserID(ctx, tx, tenantID, id); err != nil {
		return err
	}

	// FR-M005 (Module 1): converge org mappings locally (same transaction).
	// The async identity.user.deactivated event also converges this, but the
	// in-process call makes the API response truthful: by the time deactivate
	// returns 200, mappings are already inactive.
	convergersMu.RLock()
	hooks := append([]UserDeactivationConverger(nil), convergers...)
	convergersMu.RUnlock()
	for _, h := range hooks {
		if err := h.DeactivateUserMappings(ctx, tx, tenantID, id, correlationID); err != nil {
			return err
		}
	}

	eventPayload := events.UserDeactivatedPayload{
		UserID:        id,
		TenantID:      tenantID,
		DeactivatedAt: time.Now(),
	}
	evt := events.NewEvent(events.TypeUserDeactivated, events.RoutingKeyUserDeactivated, tenantID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyUserDeactivated, evt)

	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "user.deactivated", "user", id.String(), eventPayload, "", "")

	return nil
}

func mapUserToResponse(u *models.User) *dto.UserResponse {
	return &dto.UserResponse{
		ID:              u.ID.String(),
		Email:           u.Email,
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		Phone:           u.Phone,
		AvatarURL:       u.AvatarURL,
		Status:          u.Status,
		EmailVerifiedAt: u.EmailVerifiedAt,
		LastLoginAt:     u.LastLoginAt,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}
