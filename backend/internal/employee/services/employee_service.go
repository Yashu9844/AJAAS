package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/events"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	"github.com/jaas/jaas/internal/employee/validators"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

type EmployeeService interface {
	CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, req dto.UpdateEmployeeRequest) (*dto.EmployeeResponse, error)
	UpdateSelfContact(ctx context.Context, tenantID, userID uuid.UUID, req dto.UpdateSelfContactRequest) (*dto.EmployeeResponse, error)
	TransitionStatus(ctx context.Context, tenantID, id uuid.UUID, req dto.TransitionStatusRequest) (*dto.EmployeeResponse, error)
	Deactivate(ctx context.Context, tenantID, id uuid.UUID) error
}

type employeeService struct {
	db           *gorm.DB
	profileRepo  repositories.EmployeeProfileRepository
	timelineRepo repositories.EmployeeTimelineRepository
	userSvc      identityServices.UserService
	auditSvc     identityServices.AuditService
	publisher    queue.EventPublisher
}

func NewEmployeeService(
	db *gorm.DB,
	profileRepo repositories.EmployeeProfileRepository,
	timelineRepo repositories.EmployeeTimelineRepository,
	userSvc identityServices.UserService,
	auditSvc identityServices.AuditService,
	publisher queue.EventPublisher,
) EmployeeService {
	return &employeeService{
		db:           db,
		profileRepo:  profileRepo,
		timelineRepo: timelineRepo,
		userSvc:      userSvc,
		auditSvc:     auditSvc,
		publisher:    publisher,
	}
}

func (s *employeeService) CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error) {
	if err := validators.ValidateEmployeeCode(req.EmployeeCode); err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: err.Error(), StatusCode: 400}
	}
	if err := validators.ValidateEmploymentType(req.EmploymentType); err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: err.Error(), StatusCode: 400}
	}

	// 1. Verify User exists in same tenant via Module 0 UserService
	if s.userSvc != nil {
		user, err := s.userSvc.GetByID(ctx, s.db, tenantID, req.UserID)
		if err != nil || user == nil {
			return nil, sharedErrors.ErrNotFound
		}
	}

	// 2. Check for duplicate employee code
	existing, err := s.profileRepo.GetByCode(ctx, tenantID, req.EmployeeCode)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}

	// 3. Check for existing employee profile for this user
	existingUserProf, _ := s.profileRepo.GetByUserID(ctx, tenantID, req.UserID)
	if existingUserProf != nil {
		return nil, sharedErrors.ErrConflict
	}

	profileID := uuid.New()
	status := "active"
	if req.ProbationEndDate != nil && req.ProbationEndDate.After(time.Now()) {
		status = "probation"
	}

	profile := &models.EmployeeProfile{
		UserID:        req.UserID,
		EmployeeCode:  strings.ToUpper(strings.TrimSpace(req.EmployeeCode)),
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		DisplayName:   req.DisplayName,
		Gender:        req.Gender,
		DateOfBirth:   req.DateOfBirth,
		MaritalStatus: req.MaritalStatus,
		BloodGroup:    req.BloodGroup,
		AvatarURL:     req.AvatarURL,
		Status:        status,
	}
	profile.ID = profileID
	profile.TenantID = tenantID

	noticePeriod := req.NoticePeriodDays
	if noticePeriod <= 0 {
		noticePeriod = 30
	}

	employment := &models.EmploymentDetail{
		EmployeeProfileID: profileID,
		EmploymentType:    req.EmploymentType,
		JoiningDate:       req.JoiningDate,
		ProbationEndDate:  req.ProbationEndDate,
		NoticePeriodDays:  noticePeriod,
	}
	employment.TenantID = tenantID

	contact := &models.EmployeeContact{
		EmployeeProfileID: profileID,
		PersonalEmail:     req.PersonalEmail,
		WorkPhone:         req.WorkPhone,
		PersonalPhone:     req.PersonalPhone,
		CurrentAddress:    req.CurrentAddress,
		PermanentAddress:  req.PermanentAddress,
		EmergencyContacts: "[]",
	}
	contact.TenantID = tenantID

	timeline := &models.EmployeeTimeline{
		EmployeeProfileID: profileID,
		EventType:         "hired",
		EffectiveDate:     req.JoiningDate,
		Notes:             "Employee onboarded",
		Metadata:          "{}",
	}
	timeline.TenantID = tenantID

	if s.db != nil {
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			pRepo := s.profileRepo.WithTx(tx)
			if err := pRepo.Create(ctx, tx, profile); err != nil {
				return err
			}
			if err := tx.Create(employment).Error; err != nil {
				return err
			}
			if err := tx.Create(contact).Error; err != nil {
				return err
			}
			if err := tx.Create(timeline).Error; err != nil {
				return err
			}

			// Record in outbox
			payload, _ := json.Marshal(map[string]interface{}{
				"id":            profile.ID,
				"employee_code": profile.EmployeeCode,
				"user_id":       profile.UserID,
				"status":        profile.Status,
			})
			outbox := &models.EmployeeEventsOutbox{
				EventID:    uuid.New(),
				EventType:  events.EventTypeEmployeeCreated,
				RoutingKey: "employee.profile.created",
				Payload:    string(payload),
				Published:  false,
			}
			outbox.TenantID = tenantID
			return tx.Create(outbox).Error
		})
		if err != nil {
			return nil, err
		}
	} else {
		// Mock / standalone testing fallback
		_ = s.profileRepo.Create(ctx, nil, profile)
		if s.timelineRepo != nil {
			_ = s.timelineRepo.Create(ctx, nil, timeline)
		}
	}

	profile.EmploymentDetail = employment
	profile.Contact = contact

	if s.publisher != nil {
		evt := events.NewEventEnvelope(events.EventTypeEmployeeCreated, "employee.profile.created", tenantID, profile)
		_ = s.publisher.Publish(ctx, events.ExchangeName, "employee.profile.created", evt)
	}

	return toEmployeeResponse(profile), nil
}

func (s *employeeService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error) {
	profile, err := s.profileRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return toEmployeeResponse(profile), nil
}

func (s *employeeService) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return toEmployeeResponse(profile), nil
}

func (s *employeeService) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error) {
	profiles, total, err := s.profileRepo.List(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}

	res := make([]dto.EmployeeResponse, len(profiles))
	for i := range profiles {
		res[i] = *toEmployeeResponse(&profiles[i])
	}
	return res, total, nil
}

func (s *employeeService) Update(ctx context.Context, tenantID, id uuid.UUID, req dto.UpdateEmployeeRequest) (*dto.EmployeeResponse, error) {
	profile, err := s.profileRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if req.FirstName != nil {
		profile.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		profile.LastName = *req.LastName
	}
	if req.DisplayName != nil {
		profile.DisplayName = *req.DisplayName
	}
	if req.Gender != nil {
		profile.Gender = *req.Gender
	}
	if req.DateOfBirth != nil {
		profile.DateOfBirth = req.DateOfBirth
	}
	if req.MaritalStatus != nil {
		profile.MaritalStatus = *req.MaritalStatus
	}
	if req.BloodGroup != nil {
		profile.BloodGroup = *req.BloodGroup
	}
	if req.AvatarURL != nil {
		profile.AvatarURL = *req.AvatarURL
	}

	if err := s.profileRepo.Update(ctx, nil, profile); err != nil {
		return nil, err
	}

	return toEmployeeResponse(profile), nil
}

func (s *employeeService) UpdateSelfContact(ctx context.Context, tenantID, userID uuid.UUID, req dto.UpdateSelfContactRequest) (*dto.EmployeeResponse, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if profile.Contact == nil {
		profile.Contact = &models.EmployeeContact{
			EmployeeProfileID: profile.ID,
		}
		profile.Contact.TenantID = tenantID
	}

	if req.PersonalPhone != nil {
		profile.Contact.PersonalPhone = *req.PersonalPhone
	}
	if req.CurrentAddress != nil {
		profile.Contact.CurrentAddress = *req.CurrentAddress
	}
	if req.PermanentAddress != nil {
		profile.Contact.PermanentAddress = *req.PermanentAddress
	}
	if req.EmergencyContacts != nil {
		profile.Contact.EmergencyContacts = *req.EmergencyContacts
	}

	if err := s.profileRepo.UpdateContact(ctx, nil, profile.Contact); err != nil {
		return nil, err
	}

	return toEmployeeResponse(profile), nil
}

func (s *employeeService) TransitionStatus(ctx context.Context, tenantID, id uuid.UUID, req dto.TransitionStatusRequest) (*dto.EmployeeResponse, error) {
	if err := validators.ValidateStatus(req.Status); err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: err.Error(), StatusCode: 400}
	}

	profile, err := s.profileRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	oldStatus := profile.Status
	newStatus := strings.ToLower(req.Status)
	if oldStatus == newStatus {
		return toEmployeeResponse(profile), nil
	}

	profile.Status = newStatus

	if s.db != nil {
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			pRepo := s.profileRepo.WithTx(tx)
			if err := pRepo.Update(ctx, tx, profile); err != nil {
				return err
			}

			if profile.EmploymentDetail != nil {
				if req.ConfirmationDate != nil {
					profile.EmploymentDetail.ConfirmationDate = req.ConfirmationDate
				}
				if req.ResignationDate != nil {
					profile.EmploymentDetail.ResignationDate = req.ResignationDate
				}
				if req.ExitDate != nil {
					profile.EmploymentDetail.ExitDate = req.ExitDate
				}
				if req.ExitReason != "" {
					profile.EmploymentDetail.ExitReason = req.ExitReason
				}
				if err := pRepo.UpdateEmploymentDetail(ctx, tx, profile.EmploymentDetail); err != nil {
					return err
				}
			}

			// Append timeline milestone
			timeline := &models.EmployeeTimeline{
				EmployeeProfileID: profile.ID,
				EventType:         "status_changed:" + newStatus,
				EffectiveDate:     time.Now().UTC(),
				Notes:             req.Notes,
				Metadata:          "{}",
			}
			timeline.TenantID = tenantID
			return tx.Create(timeline).Error
		})
		if err != nil {
			return nil, err
		}
	} else {
		_ = s.profileRepo.Update(ctx, nil, profile)
	}

	if s.publisher != nil {
		evt := events.NewEventEnvelope(events.EventTypeEmployeeStatusChanged, "employee.status.changed", tenantID, map[string]interface{}{
			"id":         profile.ID,
			"user_id":    profile.UserID,
			"old_status": oldStatus,
			"new_status": newStatus,
		})
		_ = s.publisher.Publish(ctx, events.ExchangeName, "employee.status.changed", evt)
	}

	return toEmployeeResponse(profile), nil
}

func (s *employeeService) Deactivate(ctx context.Context, tenantID, id uuid.UUID) error {
	profile, err := s.profileRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return sharedErrors.ErrNotFound
	}
	profile.Status = "inactive"
	return s.profileRepo.Update(ctx, nil, profile)
}

func toEmployeeResponse(p *models.EmployeeProfile) *dto.EmployeeResponse {
	if p == nil {
		return nil
	}
	res := &dto.EmployeeResponse{
		ID:            p.ID,
		TenantID:      p.TenantID,
		UserID:        p.UserID,
		EmployeeCode:  p.EmployeeCode,
		FirstName:     p.FirstName,
		LastName:      p.LastName,
		DisplayName:   p.DisplayName,
		Gender:        p.Gender,
		DateOfBirth:   p.DateOfBirth,
		MaritalStatus: p.MaritalStatus,
		BloodGroup:    p.BloodGroup,
		AvatarURL:     p.AvatarURL,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}

	if p.EmploymentDetail != nil {
		res.Employment = &dto.EmploymentDetail{
			EmploymentType:   p.EmploymentDetail.EmploymentType,
			JoiningDate:      p.EmploymentDetail.JoiningDate,
			ProbationEndDate: p.EmploymentDetail.ProbationEndDate,
			ConfirmationDate: p.EmploymentDetail.ConfirmationDate,
			NoticePeriodDays: p.EmploymentDetail.NoticePeriodDays,
			ResignationDate:  p.EmploymentDetail.ResignationDate,
			ExitDate:         p.EmploymentDetail.ExitDate,
			ExitReason:       p.EmploymentDetail.ExitReason,
		}
	}

	if p.Contact != nil {
		res.Contact = &dto.EmployeeContact{
			PersonalEmail:     p.Contact.PersonalEmail,
			WorkPhone:         p.Contact.WorkPhone,
			PersonalPhone:     p.Contact.PersonalPhone,
			CurrentAddress:    p.Contact.CurrentAddress,
			PermanentAddress:  p.Contact.PermanentAddress,
			EmergencyContacts: p.Contact.EmergencyContacts,
		}
	}

	return res
}
