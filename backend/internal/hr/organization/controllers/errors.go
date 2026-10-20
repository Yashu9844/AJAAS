package controllers

import (
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

var errMissingTenant = &sharedErrors.AppError{Code: "FORBIDDEN", Message: "Missing tenant context mapping", StatusCode: 403}
