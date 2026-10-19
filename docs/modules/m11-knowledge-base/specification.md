# M11 Company Knowledge Base — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** versioned company articles, policies, taxonomy, publication
workflows, and searchable knowledge.

**Data model:** Article, ArticleVersion, Category, Tag, AudienceRule,
ReviewAssignment, PublicationState, SearchIndexRecord, Feedback.

**Representative APIs:**
- `GET /api/v1/knowledge/articles`
- `POST /api/v1/knowledge/articles`
- `GET/PATCH /api/v1/knowledge/articles/{id}`
- `POST /api/v1/knowledge/articles/{id}/submit-review`
- `POST /api/v1/knowledge/articles/{id}/publish`
- `POST /api/v1/knowledge/search`

**Rules:** immutable published versions; role/department/tenant ACL
enforced at retrieval time, not only at indexing; draft vs published
content separated; reindex events idempotent; stale index should not
leak restricted text. Link documents through Documents service
references.

**Events:** article created/updated/submitted/published/archived; index
refresh requested/completed.

**Acceptance tests:** unpublished article absent from ordinary search;
ACL-filtered search; duplicate indexing event; version history; stale
index access blocked; tenant-specific policy visibility.

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Article` | article_id, tenant_id, owner_user_id, status, current_version, classification |
| `ArticleVersion` | article_id, version, title, body_ref, created_by, created_at |
| `Category` | category_id, tenant_id, parent_id, name |
| `ArticleTag` | article_id, tag_id |
| `AudienceRule` | article_id, principal_type, principal_id, permission |
| `ReviewAssignment` | review_id, article_id, reviewer_id, status |
| `PublicationRecord` | publication_id, article_id, version, published_at, published_by |
| `SearchIndexRecord` | article_id, version, index_version, indexed_at, status |
| `ArticleFeedback` | feedback_id, article_id, user_id, score, comment_ref |

## Relationships and invariants

Article 1:N ArticleVersion, ReviewAssignment and PublicationRecord. Article M:N Tag through ArticleTag. Published versions are immutable; draft changes create a new version. Index records refer to exact article version and can be rebuilt.

## Service dependencies

M0 identity and ACL context; Documents owns linked files. Search index is a derived projection and must filter by tenant and audience at query time. AI may retrieve only through the same ACL-aware interface.
