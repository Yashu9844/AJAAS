# Module 0 — Architecture & Workflows Report

> [!TIP]
> * An editable **Draw.io** version of this architecture diagram is saved locally in this repository at [architecture.drawio](file:///c:/Users/yashwanth/Desktop/_AJASS/docs/modules/module-0-identity/architecture.drawio).
> * The raw **Mermaid** text file is saved at [architecture.mermaid](file:///c:/Users/yashwanth/Desktop/_AJASS/docs/modules/module-0-identity/architecture.mermaid). You can copy the code into any Mermaid renderer or online live editor.

This document provides a clean architectural overview, detailed system workflows, and technical rationales for **Module 0 (Tenant + Identity + RBAC)** foundation of the JAAS platform.

---

## 1. Clean Architectural Diagram

The system is designed following **Clean Architecture** principles. Dependencies flow inward: HTTP and infrastructure layers depend on services, which depend on repositories and domain models.

```mermaid
flowchart TB
    %% Class Definitions for Premium Styling
    classDef client fill:#E1F5FE,stroke:#039BE5,stroke-width:2px,color:#01579B;
    classDef middleware fill:#FFF3E0,stroke:#FB8C00,stroke-width:2px,color:#E65100;
    classDef controller fill:#FFEBEE,stroke:#E53935,stroke-width:2px,color:#B71C1C;
    classDef service fill:#E8F5E9,stroke:#43A047,stroke-width:2px,color:#1B5E20;
    classDef repo fill:#F3E5F5,stroke:#8E24AA,stroke-width:2px,color:#4A148C;
    classDef storage fill:#ECEFF1,stroke:#546E7A,stroke-width:2px,color:#263238;

    %% Subgraphs for Structured Layers
    subgraph ClientLayer ["🌐 Request Ingress"]
        Client["Client (Web / Mobile App)"]:::client
    end

    subgraph MiddlewareLayer ["🛡️ Security & Context Middleware Stack"]
        CORS["CORS Middleware"]:::middleware
        RateLimit["Rate Limiter (Redis-Backed)"]:::middleware
        TenantResolver["Tenant Resolver (Subdomain Parse)"]:::middleware
        AuthMiddleware["JWT Authenticator"]:::middleware
        RBAC["RBAC Permission Guard"]:::middleware

        CORS --> RateLimit
        RateLimit --> TenantResolver
        TenantResolver --> AuthMiddleware
        AuthMiddleware --> RBAC
    end

    subgraph DeliveryLayer ["⚡ Delivery / API Handlers"]
        Controllers["Gin REST Controllers"]:::controller
    end

    subgraph DomainLayer ["🧠 Services Business Logic"]
        Services["Core Services Layer"]:::service
    end

    subgraph InfraLayer ["💾 Infrastructure Access"]
        Repos["GORM Repositories Layer"]:::repo
    end

    subgraph StorageLayer ["🔋 Storage & Message Brokers"]
        Postgres[("🐘 PostgreSQL DB\n(Persistent Models)")]:::storage
        Redis[("⚡ Redis Cache\n(Blacklists & Rate Limits)")]:::storage
        RabbitMQ[("🐇 RabbitMQ Broker\n(Async Event Queue)")]:::storage
    end

    %% Cross-Layer Flow Mappings
    Client -->|HTTP Request| CORS
    RBAC -->|Validated Context| Controllers
    Controllers -->|Invoke Action| Services
    Services -->|Database Calls| Repos
    Repos -->|Write/Read| Postgres
    Services -->|Session / Rate State| Redis
    Services -->|Publish Events| RabbitMQ
```

---

## 2. Core Workflows

### A. Tenant Signup & Onboarding Workflow

This flow ensures new organizations register cleanly with automatic system roles provisioned.

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant Controller as Tenant Controller
    participant Service as Tenant Service
    participant Repo as Tenant Repository
    participant RoleRepo as Role Repository
    participant Broker as RabbitMQ Broker
    participant DB as PostgreSQL

    Client->>Controller: POST /tenants (name, slug, domain, plan)
    Controller->>Service: CreateTenant(req, correlationID)
    
    Note over Service: 1. Validate Slug regex format<br/>2. Block reserved subdomains (admin, api, etc.)
    
    Service->>Repo: Check Slug uniqueness in database
    Repo->>DB: SELECT FROM tenants WHERE slug = ?
    DB-->>Repo: No duplicates
    
    Service->>Repo: Create Tenant record (Status: Active)
    Repo->>DB: INSERT INTO tenants (...)
    DB-->>Repo: Saved (returns tenant ID)
    
    Note over Service: 3. Provision default system roles: tenant_admin and member
    Service->>RoleRepo: Create system roles
    RoleRepo->>DB: INSERT INTO roles (tenant_id, name, is_system: true)
    
    Service->>Broker: Publish "TenantCreated" event (async confirm)
    Service->>DB: Write Immutable Audit Log (tenant.created)
    
    Service-->>Controller: Return TenantResponse DTO
    Controller-->>Client: 201 Created (Success JSON)
```

---

### B. User Signin & Authentication Workflow

This flow authenticates users, starts active sessions, and deploys security tokens (JWT + Opaque Refresh).

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant Controller as Auth Controller
    participant Service as Auth Service
    participant SessionSvc as Session Service
    participant TokenSvc as Token Service
    participant DB as PostgreSQL
    participant Cache as Redis Store

    Client->>Controller: POST /auth/login (email, password, tenant_slug)
    Controller->>Service: Login(req, ipAddress, userAgent)
    
    Service->>DB: Resolve Tenant by slug
    DB-->>Service: Active Tenant
    Note over Service: Block logins if Tenant status is suspended
    
    Service->>DB: Resolve User by email + tenant_id
    DB-->>Service: Active User & password_hash
    Note over Service: Block logins if User status is inactive or invited
    
    Note over Service: Verify bcrypt password hash (cost 12)
    
    Service->>SessionSvc: Create active Session
    SessionSvc->>DB: INSERT INTO sessions (expires_at: 15m)
    
    Service->>TokenSvc: Generate Opaque Refresh Token
    TokenSvc-->>Service: Return raw token & SHA-256 hash
    Service->>DB: INSERT INTO refresh_tokens (token_hash, expires_at: 7d)
    
    Service->>TokenSvc: Encode HS256 Access JWT<br/>(claims: sub, tid, email, roles, sid)
    
    Service->>DB: Update User last_login_at
    Service->>DB: Write Audit Log (login.success)
    
    Service-->>Controller: Return LoginResponse DTO
    Controller-->>Client: 200 OK (access_token, refresh_token, user details)
```

---

### C. Refresh Token Rotation & Reuse Detection (RT-005)

To prevent session hijacks, we rotate refresh tokens on every use and implement absolute reuse detection.

```mermaid
graph TD
    A[Client Requests /auth/refresh] --> B(Hash incoming raw Refresh Token)
    B --> C{Look up token_hash in DB}
    C -->|Not Found or Expired| D[Return 401 Unauthorized]
    C -->|Found| E{Is token already revoked / used?}
    E -->|Yes: Breach Detected| F[Security Wipe: Invalidate ALL active sessions and refresh tokens for this user]
    F --> G[Log Token Reuse Audit Log & Return 401]
    E -->|No: Valid Token| H[Verify User and Tenant Status are Active]
    H -->|Expired/Blocked| D
    H -->|Active| I[Rotate Tokens: Mark old token as revoked]
    I --> J[Generate NEW Opaque Refresh Token & Hash]
    J --> K[Generate NEW Session & HS256 Access JWT]
    K --> L[Save to DB & Return Token Pair to Client]
```

---

## 3. Technology Stack Rationale

### A. Why Redis?
1. **Low-Latency Session Verification**: Rather than hitting PostgreSQL on every API call to confirm a user's session validity, the [auth](file:///c:/Users/yashwanth/Desktop/_AJASS/backend/internal/identity/middleware/auth.go) middleware queries Redis for blacklisted sessions.
2. **Instant JWT Blacklisting**: On logout, access tokens cannot be deleted from a client's device, so their `sid` (session ID) is blacklisted in Redis with a 15-minute TTL (matching access token expiration), resolving stateless logout invalidation.
3. **High-Performance Counters**: Sliding window rate limits require fast memory operations that Redis handles natively via atomic key incrementation.

### B. Why RabbitMQ?
1. **Decoupled Architecture**: When a tenant or user is created, notifications (emails, SMS) or downstream sync services need to run. RabbitMQ allows the Identity service to publish a lightweight event and exit immediately without waiting for secondary processes.
2. **Reliability & Eventual Consistency**: RabbitMQ features **Publish Confirms** and durable queues. If downstream services (e.g. notification consumer) are down, events are safe in the broker and processed as soon as consumers reconnect.
3. **Asynchronous Audit Scopes**: Immutable security audit logs are written to the database asynchronously via events queue consumers, protecting the primary API thread from DB write latencies.

### C. Why Rate Limiters?
1. **Brute-Force Attack Prevention**: Limits access to critical authorization endpoints (`/auth/login`, `/auth/forgot-password`). Brute-forcing passwords is capped at 10 requests per 15 minutes, blocking bots.
2. **Resource Exhaustion Mitigations**: Prevents bad actors from spawning high volumes of costly cryptographic actions (like Bcrypt password hashes which run at work cost 12, taking significant CPU cycles).
3. **Fail-Safe Fallback**: If Redis experiences connection outages, the rate limiter middleware is designed to **fail-open**—logging warning signals but allowing legitimate requests to bypass, prioritizing platform availability.
