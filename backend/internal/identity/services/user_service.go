package services

import (
	"context"
	"fmt"
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
}

type userService struct {
	userRepo     repositories.UserRepository
	roleRepo     repositories.RoleRepository
	userRoleRepo repositories.UserRoleRepository
	sessionRepo  repositories.SessionRepository
	tokenRepo    repositories.RefreshTokenRepository
	publisher    queue.EventPublisher
	auditSvc     AuditService
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
) UserService {
	return &userService{
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		userRoleRepo: userRoleRepo,
		sessionRepo:  sessionRepo,
		tokenRepo:    tokenRepo,
		publisher:    publisher,
		auditSvc:     auditSvc,
	}
}

func (s *userService) CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error) {
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

	// Map and associate default roles
	var assignedRoleIDs []uuid.UUID
	if len(req.RoleIDs) > 0 {
		for _, rIDStr := range req.RoleIDs {
			roleID, err := uuid.Parse(rIDStr)
			if err != nil {
				return nil, sharedErrors.ErrValidation
			}

			// Verify role exists in this tenant
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
		for _, rIDStr := range req.RoleIDs {
			roleID, err := uuid.Parse(rIDStr)
			if err != nil {
				return nil, sharedErrors.ErrValidation
			}

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
	return mapUserToResponse(user), nil
}

func (s *userService) ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error) {
	users, total, err := s.userRepo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
		return nil, err
	}

	data := make([]dto.UserResponse, len(users))
	for i := range users {
		data[i] = *mapUserToResponse(&users[i])
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

	return mapUserToResponse(user), nil
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
