# marketplace-api

Public, read-only marketplace catalog + curation API for Construct spaces.

Owns the live catalog the desktop app browses (categories, tags, staff
collections, trending) plus the internal promote/demote hand-off from
developer-api when a publish is approved.

## Layout

```
internal/
  config/         env-var loader
  database/       gorm + Postgres + raw migrations (GIN indexes, pg_trgm)
  gwauth/         gateway-trust helper (X-Internal-Secret)
  handlers/
    health.go         GET /health
    marketplace.go    public read-only routes
    internal.go       /internal/* called by developer-api + desktop
    admin.go          /api/admin/* curation surface for oracle-web staff UI
  middleware/     CORS, logger, AdminAuth (gateway-secret), ratelimit
  models/         Space, Category, Collection, CollectionSpace,
                  EditorialOverride, InstallEvent
```

## Routes

### Public (`/api/marketplace/*`)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/marketplace/spaces?q=&category=&tag=&page=&pageSize=&sort=` | Paginated catalog |
| GET | `/api/marketplace/spaces/{name}` | Detail |
| GET | `/api/marketplace/categories` | Category enum |
| GET | `/api/marketplace/collections` | Active curated lists |
| GET | `/api/marketplace/collections/{slug}` | Collection + spaces |

CORS open. Cache `public, max-age=60`. No auth required.

### Internal (`/internal/*`, gateway-secret gated)

| Method | Path | Caller |
|--------|------|--------|
| POST | `/internal/spaces` | developer-api on approve (upsert) |
| DELETE | `/internal/spaces/{name}` | developer-api on unpublish (soft delete) |
| POST | `/internal/install-events` | desktop on successful install |

### Admin (`/api/admin/*`, gateway-secret gated)

The full staff curation surface — consumed by **oracle-web**. Same
`X-Internal-Secret` as the `/internal/*` lane; the split is by
**audience** (operators write here, services write to `/internal`),
not by trust level.

#### Spaces

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/admin/spaces` | List all spaces (incl. unpublished) |
| PATCH | `/api/admin/spaces/{name}` | Edit category, tags, visibility |
| GET | `/api/admin/spaces/{name}/editorial` | Read editorial override (hero copy, screenshots, featured flag) |
| PUT | `/api/admin/spaces/{name}/editorial` | Upsert editorial override |

`EditorialOverride` is a sibling row keyed by space name — staff edit
it without touching the developer-owned manifest.

#### Categories

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/admin/categories` | List all (admin lane returns hidden + ordering metadata) |
| POST | `/api/admin/categories` | Create |
| PUT | `/api/admin/categories/{slug}` | Rename, recolor, reorder |
| DELETE | `/api/admin/categories/{slug}` | Delete (rejects if in use) |

#### Collections

Curated lists ("Editor's picks", "New this week", etc.). Each
collection has an ordered set of spaces.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/admin/collections` | List all (incl. inactive) |
| GET | `/api/admin/collections/{id}` | Detail + ordered spaces |
| POST | `/api/admin/collections` | Create |
| PUT | `/api/admin/collections/{id}` | Edit title, slug, description, active flag |
| DELETE | `/api/admin/collections/{id}` | Delete |
| POST | `/api/admin/collections/{id}/spaces` | Add a space to a collection |
| DELETE | `/api/admin/collections/{id}/spaces/{name}` | Remove a space |
| PUT | `/api/admin/collections/{id}/order` | Reorder spaces in a collection |

## Environment

```
PORT=8000
APP_URL=http://localhost:8000

DB_HOST=srv-captain--postgres-db
DB_PORT=5432
DB_NAME=marketplace
DB_USER=marketplace_app
DB_PASS=<from CapRover>

DEVELOPER_URL=http://srv-captain--developer-api
GRAPH_URL=http://srv-captain--graph
INTERNAL_SHARED_SECRET=<shared with my.lisaos.dev gateway>

ALLOWED_ORIGINS=https://my.lisaos.dev,tauri://localhost,...
HOME_CACHE_TTL=60
```

## Run locally

```
docker run -d --name pg17 -p 5432:5432 \
  -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=marketplace postgres:17.9-alpine
DB_HOST=localhost DB_PASS=dev DB_USER=postgres go run .
```

Then:

```
curl http://localhost:8000/health
curl http://localhost:8000/api/marketplace/spaces
```
