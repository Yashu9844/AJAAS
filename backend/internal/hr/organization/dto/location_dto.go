package dto

import "time"

// CreateLocationRequest creates a work site. FR-L001.
type CreateLocationRequest struct {
	Code         string  `json:"code" binding:"required,min=2,max=32"`
	Name         string  `json:"name" binding:"required,min=1,max=150"`
	AddressLine1 *string `json:"address_line1" binding:"omitempty,max=200"`
	AddressLine2 *string `json:"address_line2" binding:"omitempty,max=200"`
	City         string  `json:"city" binding:"required,min=1,max=100"`
	StateCode    string  `json:"state_code" binding:"required,min=1,max=10"`
	CountryCode  *string `json:"country_code" binding:"omitempty,len=2"`
	PostalCode   *string `json:"postal_code" binding:"omitempty,max=20"`
	TimeZone     *string `json:"time_zone" binding:"omitempty,max=64"`
}

// UpdateLocationRequest patches a location; code is immutable (a different code is rejected). FR-L003.
type UpdateLocationRequest struct {
	Code         *string `json:"code" binding:"omitempty,min=2,max=32"`
	Name         *string `json:"name" binding:"omitempty,min=1,max=150"`
	AddressLine1 *string `json:"address_line1" binding:"omitempty,max=200"`
	AddressLine2 *string `json:"address_line2" binding:"omitempty,max=200"`
	City         *string `json:"city" binding:"omitempty,min=1,max=100"`
	StateCode    *string `json:"state_code" binding:"omitempty,min=1,max=10"`
	CountryCode  *string `json:"country_code" binding:"omitempty,len=2"`
	PostalCode   *string `json:"postal_code" binding:"omitempty,max=20"`
	TimeZone     *string `json:"time_zone" binding:"omitempty,max=64"`
}

// LocationFilter narrows location lists. FR-L002.
type LocationFilter struct {
	Status    string
	StateCode string
	Search    string
}

// LocationResponse serializes a location.
type LocationResponse struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	AddressLine1 *string   `json:"address_line1,omitempty"`
	AddressLine2 *string   `json:"address_line2,omitempty"`
	City         string    `json:"city"`
	StateCode    string    `json:"state_code"`
	CountryCode  string    `json:"country_code"`
	PostalCode   *string   `json:"postal_code,omitempty"`
	TimeZone     string    `json:"time_zone"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LocationListResponse is a paginated location list.
type LocationListResponse struct {
	Data []LocationResponse `json:"data"`
	Meta PaginationMeta     `json:"meta"`
}
