package models

import "time"

var NeverExpiresAt = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

type Share struct {
	ID                 string
	Title              string
	FileName           string
	MimeType           string
	StoragePath        string
	AccessPasswordHash string
	PickcodeEncrypted  string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	DeletedAt          *time.Time
	Files              []ShareFile
}

type ShareFile struct {
	ID          string
	ShareID     string
	FileName    string
	MimeType    string
	StoragePath string
	Size        int64
	CreatedAt   time.Time
}

func (s Share) HasPassword() bool {
	return s.AccessPasswordHash != ""
}

func (s Share) NeverExpires() bool {
	return IsNeverExpiresTime(s.ExpiresAt)
}

func (s Share) IsExpired(now time.Time) bool {
	if s.NeverExpires() {
		return false
	}
	return !s.ExpiresAt.After(now)
}

func (s Share) IsDeleted() bool {
	return s.DeletedAt != nil
}

func (s Share) Status(now time.Time) string {
	if s.IsDeleted() {
		return "deleted"
	}
	if s.IsExpired(now) {
		return "expired"
	}
	return "active"
}

type AdminSession struct {
	ID        string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type AccessLog struct {
	ID         string
	ShareID    string
	Action     string
	IP         string
	UserAgent  string
	StatusCode int
	CreatedAt  time.Time
}

type ShareInput struct {
	Title          string
	AccessPassword string
	ExpiresHours   int
}

type ShareSummary struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	FileName      string      `json:"fileName"`
	HasPassword   bool        `json:"hasPassword"`
	ExpiresAt     time.Time   `json:"expiresAt"`
	CreatedAt     time.Time   `json:"createdAt"`
	Status        string      `json:"status"`
	ShareURL      string      `json:"shareUrl"`
	Files         []ShareFile `json:"files"`
	FileCount     int         `json:"fileCount"`
	TotalSize     int64       `json:"totalSize"`
	DownloadCount int64       `json:"downloadCount"`
	DeletedAt     *time.Time  `json:"deletedAt,omitempty"`
}

func IsNeverExpiresTime(value time.Time) bool {
	return value.UTC().Year() >= NeverExpiresAt.Year()
}
