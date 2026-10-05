# Module 2 — Assumptions

1. **User Exists in Identity:** An employee profile must be associated with an existing, active `User` in the same tenant (Module 0).
2. **Document Binary Storage:** In v1, employee documents store URL references (`file_url`), sizes, and MIME types. Cloud object storage (S3/GCS/MinIO) generates pre-signed URLs or upload targets.
3. **Org Mapping Independence:** Module 1 manages which department/team/manager an employee belongs to; Module 2 manages the person's employment terms, contact information, and documents.
