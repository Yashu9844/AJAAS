package collab

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaas/jaas/internal/projects/kernel"
)

// Attachment limits (AT-01, AT-02).
const (
	MaxFileBytes      = 50 << 20
	TenantQuotaBytes  = 5 << 30
	MaxPerTask        = 200
	UploadTTL         = 5 * time.Minute
	DownloadTTL       = 2 * time.Minute
	PendingMaxAge     = 24 * time.Hour
	maxFilenameLength = 255
	sweepBatch        = 500
)

var allowedTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true, "text/plain": true, "text/csv": true, "text/markdown": true, "application/json": true,
	"application/zip": true, "video/mp4": true, "audio/mpeg": true,
	"application/msword":       true,
	"application/vnd.ms-excel": true, "application/vnd.ms-powerpoint": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.oasis.opendocument.text":                                   true,
	"application/vnd.oasis.opendocument.spreadsheet":                            true,
	"application/vnd.oasis.opendocument.presentation":                           true,
}

var executableExt = map[string]bool{
	"exe": true, "dll": true, "bat": true, "cmd": true, "com": true, "msi": true, "scr": true, "ps1": true, "psm1": true,
	"vbs": true, "vbe": true, "js": true, "jse": true, "wsf": true, "wsh": true, "hta": true, "jar": true, "sh": true,
	"bash": true, "apk": true, "app": true, "dmg": true, "pkg": true, "deb": true, "rpm": true, "lnk": true, "reg": true,
	"cpl": true, "msc": true, "pif": true, "gadget": true, "bin": true, "run": true, "elf": true,
}

var sha256Hex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// sanitizeFilename keeps the last path segment, drops control and format characters (bidi overrides included) and rejects
// names that are empty after that, longer than 255 characters or carry an executable extension anywhere after the first dot.
func sanitizeFilename(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", kernel.Invalid("filename", "must be valid text")
	}
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	name := strings.TrimFunc(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw), func(r rune) bool { return unicode.IsSpace(r) || r == '.' })
	if name == "" {
		return "", kernel.Invalid("filename", "is required")
	}
	if utf8.RuneCountInString(name) > maxFilenameLength {
		return "", kernel.Invalid("filename", "is longer than 255 characters")
	}
	parts := strings.Split(strings.ToLower(name), ".")
	for _, ext := range parts[1:] {
		if executableExt[strings.TrimSpace(ext)] {
			return "", kernel.Err(400, "FILE_TYPE_NOT_ALLOWED", "executable files cannot be attached")
		}
	}
	return name, nil
}

// normalizeContentType lowercases the media type and drops parameters, then checks the allow-list.
func normalizeContentType(raw string) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if !allowedTypes[ct] {
		return "", kernel.Err(400, "FILE_TYPE_NOT_ALLOWED", "this content type cannot be attached")
	}
	return ct, nil
}

// InitInput describes the file a client wants to upload.
type InitInput struct {
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
}

// checked is an InitInput after sanitising and validation.
type checked struct {
	filename, contentType, sha string
	size                       int64
}

func (in InitInput) validate() (checked, error) {
	name, err := sanitizeFilename(in.Filename)
	if err != nil {
		return checked{}, err
	}
	ct, err := normalizeContentType(in.ContentType)
	if err != nil {
		return checked{}, err
	}
	if in.Size < 1 {
		return checked{}, kernel.Invalid("size", "must be at least 1 byte")
	}
	if in.Size > MaxFileBytes {
		return checked{}, kernel.Err(400, "FILE_TOO_LARGE", "files are limited to 50 MB")
	}
	sum := strings.ToLower(in.SHA256)
	if sum != "" && !sha256Hex.MatchString(sum) {
		return checked{}, kernel.Invalid("sha256", "must be 64 hexadecimal characters")
	}
	return checked{filename: name, contentType: ct, sha: sum, size: in.Size}, nil
}
