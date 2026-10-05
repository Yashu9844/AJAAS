package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type userRepository struct{}

// NewUserRepository returns a UserRepository.
func NewUserRepository() UserRepository {
	return &userRepository{}
}

func (r *userRepository) Create(ctx context.Context, tx *gorm.DB, user *models.User) error {
	return tx.WithContext(ctx).Create(user).Error
}

func (r *userRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := db.WithContext(ctx).First(&user, "tenant_id = ? AND id = ?", tenantID, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error) {
	var user models.User
	err := db.WithContext(ctx).First(&user, "tenant_id = ? AND email = ?", tenantID, email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	query := db.WithContext(ctx).Model(&models.User{}).Where("tenant_id = ?", tenantID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	err := query.Offset(offset).Limit(perPage).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *userRepository) Update(ctx context.Context, tx *gorm.DB, user *models.User) error {
	return tx.WithContext(ctx).Where("tenant_id = ? AND id = ?", user.TenantID, user.ID).Save(user).Error
}

func (r *userRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.User{}, "tenant_id = ? AND id = ?", tenantID, id).Error
}
