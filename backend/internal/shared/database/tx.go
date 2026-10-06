package database

import (
	"context"

	"gorm.io/gorm"
)

// BeginTx starts a request-scoped transaction. A nil db (unit tests with mocked services) yields a nil tx,
// which services treat the same way they treat the nil db they were previously handed.
func BeginTx(ctx context.Context, db *gorm.DB) *gorm.DB {
	if db == nil {
		return nil
	}
	return db.WithContext(ctx).Begin()
}

// Finish commits tx when err is nil and rolls back otherwise. It returns the error the caller must report
// (the original err, or the commit failure). Safe to call with a nil tx.
func Finish(tx *gorm.DB, err error) error {
	if tx == nil {
		return err
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}
