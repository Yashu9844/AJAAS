-- HR/Learning catalogue (LN-C01, LN-C02, LN-C09).
CREATE TABLE lrn_settings (
    tenant_id UUID PRIMARY KEY,
    time_zone VARCHAR(64) NOT NULL,
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE lrn_courses (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title VARCHAR(150) NOT NULL,
    description VARCHAR(2000),
    provider VARCHAR(100),
    content_kind VARCHAR(10) NOT NULL CHECK (content_kind IN ('link', 'document')),
    content_url VARCHAR(500),
    document_ref VARCHAR(200),
    thumbnail_ref VARCHAR(500),
    duration_mins INT NOT NULL DEFAULT 0 CHECK (duration_mins BETWEEN 0 AND 10000),
    tags JSONB NOT NULL DEFAULT '[]',
    verification VARCHAR(10) NOT NULL DEFAULT 'self' CHECK (verification IN ('self', 'manager', 'hr')),
    self_enrol BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(10) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    version INT NOT NULL DEFAULT 1,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT lrn_courses_content CHECK (
        (content_kind = 'link' AND content_url IS NOT NULL AND document_ref IS NULL) OR
        (content_kind = 'document' AND document_ref IS NOT NULL AND content_url IS NULL))
);
CREATE INDEX idx_lrn_courses_tenant ON lrn_courses (tenant_id, status);

CREATE TABLE lrn_course_versions (
    tenant_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES lrn_courses(id) ON DELETE CASCADE,
    version INT NOT NULL,
    snapshot JSONB NOT NULL,
    changed_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (course_id, version)
);
