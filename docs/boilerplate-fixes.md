# Boilerplate Fixes Log

Dokumentasi error yang ditemui saat pertama kali coba `make run` di project ini,
plus root cause + fix-nya. Disimpan supaya kalau ke depan ada onboarding atau
issue serupa, tinggal dibaca.

Status setiap fix sudah di-port ke template generator (`internal/template/files/`)
per 2026-05-03 — project baru hasil `chasago init` sudah aman dari issue 1-9.

---

## 1. fx — `missing type: router.Deps`

### Gejala

```
ERROR  fx  invoke failed
  error: missing dependencies for function "router.Register":
  missing type: router.Deps (did you mean to Provide it?)
```

Server gagal start saat fx mau invoke `router.Register`.

### Root Cause

`router.Register(d Deps)` menerima parameter berupa **struct** (`Deps`). Uber Fx
**tidak otomatis** membangun struct dari komponen-komponen yang sudah di-provide
ke graph. Untuk membuat fx bisa fill field-field-nya, struct harus secara
eksplisit menandakan dirinya sebagai *parameter object* dengan embed `fx.In`.

Boilerplate awal (sejak first commit) tidak punya `fx.In` di `router.Deps`,
jadi memang **belum pernah berhasil dijalankan**.

### Fix

`internal/app/router/router.go`:

```go
import (
    // ...
    "go.uber.org/fx"
)

type Deps struct {
    fx.In  // <— tambahkan ini

    Engine        *gin.Engine
    Cfg           *config.Config
    Logger        *zap.Logger
    Redis         *redis.Client
    Paseto        *token.Paseto
    UserCtrl      *controller.UserController
    AuthCtrl      *controller.AuthController
    AdminUserCtrl *controller.AdminUserController
}
```

### Pelajaran

Kalau di project ini ada constructor lain yang menerima struct sebagai parameter
(bukan field individual), pastikan struct-nya embed `fx.In`. Alternatif: bikin
constructor terpisah yang mengembalikan `Deps` lalu `fx.Provide(NewDeps)` —
tapi `fx.In` jauh lebih ringkas.

---

## 2. golang-migrate — `init migrate driver: no schema`

### Gejala

```
ERROR  fx  OnStart hook failed
  error: init migrate driver: no schema
```

Auto-migrate gagal saat startup, walaupun database connection sudah berhasil
(server sempat log `http server listening`).

### Root Cause

golang-migrate v4 driver `postgres` saat init memanggil `SELECT CURRENT_SCHEMA()`
ke database. Kalau hasilnya NULL (search_path tidak ter-resolve ke schema
manapun), driver melempar `ErrNoSchema`.

Project ini pakai `pgx/v5/stdlib` sebagai SQL driver, dan DSN-nya:

```
postgres://user:pass@host:5432/db?sslmode=disable
```

**Tidak ada `search_path` parameter.** Dengan pgx, `CURRENT_SCHEMA()` bisa
return NULL di kondisi tertentu (terutama jika role default search_path-nya
override atau koneksi pool reset). Hasilnya migrate driver gagal init.

### Fix (pilih salah satu)

**Opsi A — di DB (rekomendasi untuk dev lokal, sekali set):**

```sql
ALTER ROLE postgres SET search_path = public;
-- atau spesifik per database
ALTER DATABASE jastipchasastore_dev SET search_path = public;
```

Setelah itu reconnect — `CURRENT_SCHEMA()` resolve ke `public` dan migrate
happy. Cara ini tidak mengubah codebase.

**Opsi B — di DSN (sudah diterapkan di codebase ini):**

Ada **dua tempat** DSN — keduanya wajib include `search_path=public`:

```go
// internal/platform/config/config.go (Go runtime, AutoUp)
return fmt.Sprintf(
    "postgres://%s:%s@%s:%d/%s?sslmode=%s&search_path=public",
    d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
)
```

```makefile
# Makefile (CLI: make migrate-up, make db-reset, dll)
DB_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)&search_path=public
```

Kalau hanya salah satu yang di-fix, perintah lewat jalur yang lain akan error
`no schema`. Pastikan **selalu sinkron**.

### Pelajaran

Kalau pakai `pgx` + golang-migrate `postgres` driver, **selalu** set
`search_path` eksplisit di DSN. Ini behaviour yang berbeda dari `lib/pq` yang
biasanya implicit fallback ke `public`.

---

## 3. Schema vs Data: `roles` di seeder, bukan migration

### Gejala (historis)

```
ERROR  fx  OnStart hook failed
  error: migrate up: migration failed in line 0: ...DELETE FROM roles WHERE name = 'guest';
  (details: ERROR: relation "roles" does not exist (SQLSTATE 42P01))
```

Migration `20260430120000_drop_guest_role.up.sql` gagal di DB fresh karena
referensi ke table `roles` yang belum ada — table itu dibuat oleh seeder, bukan
migration.

### Root Cause

Boilerplate awal punya **inkonsistensi arsitektur**: table `users`,
`refresh_tokens`, `audit_logs` dibuat lewat **migration**, tapi table `roles`
(lookup table) dibuat lewat **seeder** (`role_seeder.go`):

```go
const ddl = `CREATE TABLE IF NOT EXISTS roles (...)`
```

Hasilnya:

- Fresh DB tanpa `make seed` → server start → migrations run → migration
  yang reference `roles` fail karena table belum ada.
- Setelah `make seed` → `roles` ada, migrasi jalan normal.

Urutan tergantung: pertama kali run pakai `make run` (bukan `make seed`),
startup gagal.

### Fix yang diterapkan di template

**Schema = migration. Data = seeder.** DDL `roles` dipindah ke migration
(`20260424120010_create_table_roles.up.sql`); seeder hanya `INSERT`/`UPSERT`
data default. Lihat juga §8 untuk per-table migration policy.

```go
// internal/database/seeders/role_seeder.go — insert-only sekarang
INSERT INTO roles (name, description) VALUES (...)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;
```

### Pelajaran

Kalau seeder bikin DDL (`CREATE TABLE`), refactor ASAP — jadi sumber bug
yang aneh. Migration handle struktur, seeder handle data. Tidak overlap.

---

## 4. Migration Dirty State Recovery

### Kapan Terjadi

Kalau salah satu migration di atas gagal di tengah jalan, golang-migrate akan
menandai schema_migrations row dengan `dirty=true` untuk versi terakhir yang
dicoba. Server berikutnya start akan terus error:

```
ERROR  Dirty database version 20260430120000. Fix and force version.
```

### Cara Recovery (1-liner)

```bash
psql -U postgres -d jastipchasastore_dev -c \
  "UPDATE schema_migrations SET version = 20260424120000, dirty = false;"
```

- Reset version ke `init_schema` (pasti sudah ke-apply)
- Lepas flag `dirty`
- `make run` berikutnya akan retry migration yang gagal

### Cara Recovery (alternatif via CLI)

Kalau punya golang-migrate CLI (`brew install golang-migrate`):

```bash
migrate -database "$DSN" -path ./internal/database/migrations \
        force 20260424120000
migrate -database "$DSN" -path ./internal/database/migrations up
```

### Cek State Manual

```sql
SELECT * FROM schema_migrations;
-- version | dirty
-- 20260430120000 | t   ← dirty=true berarti gagal di tengah
```

---

## 5. `make migrate-up` — `unknown driver postgres (forgotten import?)`

### Gejala

```
$ make migrate-up
error: failed to open database: database driver: unknown driver postgres (forgotten import?)
```

### Root Cause

`golang-migrate` CLI di-install **tanpa** build tag `postgres`. Driver postgres
adalah **opt-in**: harus eksplisit di-include saat compile, kalau tidak CLI
tidak tahu cara konek ke Postgres walau syntax-nya benar.

### Fix

Jalankan target `tools` di Makefile (sudah disiapkan untuk install tepat):

```bash
make tools
```

Yang dijalankan:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/air-verse/air@latest
```

`-tags 'postgres'` itu yang menentukan. Setelah ini `make migrate-up` jalan.

### Pelajaran

Kalau install CLI dari source (bukan dari brew/apt), perhatikan apakah ada
build tag yang harus di-include. golang-migrate dukung banyak driver — semuanya
opt-in via tag.

---

## 6. Laravel `migrate:fresh --seed` Equivalent

### Pertanyaan

> Di Laravel ada `php artisan migrate:fresh --seed` untuk drop semua + migrate
> ulang + seed. Di Go ada cara serupa?

### Jawaban

Ada — `make db-reset` (lihat `Makefile`):

```makefile
db-reset:
    PGPASSWORD=$(DB_PASSWORD) psql ... -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
    @$(MAKE) migrate-up
    @$(MAKE) seed
```

Pakai untuk:
- Recovery total saat dirty migration + ingin start ulang
- Reset DB ke state bersih saat dev (mirip `npm run reset`)
- **Jangan dijalankan di production** — drop schema = data hilang

### Pelajaran

Setiap kali boilerplate Go yang punya migration + seeder, pasti useful punya
target `db-reset` atau `db-fresh`. Kalau project lain belum ada, copy pattern-
nya dari sini.

---

## 7. Auth Strict Cookie-Only (jangan pakai Bearer)

### Konteks

Boilerplate awal menggunakan **dual mode auth**: middleware baca cookie dulu,
lalu fallback ke `Authorization: Bearer <token>` header. Login response juga
return tokens di body (`access_token`, `refresh_token`, `expires_in`,
`token_type`). Tujuannya supaya Postman/mobile/CLI klien bisa pegang token
langsung.

Tapi praktiknya menimbulkan masalah:

- **Dua jalur untuk satu fitur** — middleware, controller, dan FE harus
  handle dua kondisi (cookie ada / tidak ada → fallback header). Lebih banyak
  branching, lebih rentan bug autentikasi yang silently lewat.
- **Token leakage risk** — kalau body response masih kasih token, FE yang
  ceroboh bisa simpan ke `localStorage` (rentan XSS), padahal cookie HttpOnly
  sudah cukup.
- **Spec membingungkan** — OpenAPI punya 2 `securitySchemes` yang dua-duanya
  "valid"; reviewer/integrator bingung mana yang harus dipakai.
- **Inconsistency antara admin dan customer FE** — kalau salah satu pakai
  Bearer, satu lagi pakai cookie, code-path-nya beda padahal endpoint sama.

### Keputusan

**Strict cookie-only.** Single flow untuk admin & customer:

```
POST /api/auth/login
   ↓ Set-Cookie: access_token (HttpOnly, Path=/api)
   ↓ Set-Cookie: refresh_token (HttpOnly, Path=/api/auth)
   ↓ body response: {meta, data: []}        ← kosong, by design
   
GET /api/me                  ← klien fetch info user di sini
   ↓ Cookie: access_token=...
```

Tidak ada Authorization Bearer di mana pun. Klien non-browser harus pakai
**cookie jar** (`curl -c/-b cookies.txt`, Postman cookie support, dll).

### Cookie attributes (wajib)

```go
http.SetCookie(c.Writer, &http.Cookie{
    Name:     "access_token",            // atau "refresh_token"
    Value:    token,
    Path:     "/api",                    // refresh: "/api/auth"
    HttpOnly: true,                      // wajib — JS tidak boleh akses
    Secure:   cfg.App.IsProduction(),    // wajib true di prod (HTTPS only)
    SameSite: http.SameSiteLaxMode,      // CSRF protection minimal
    MaxAge:   int(ttl.Seconds()),
})
```

### Fix yang diterapkan di template

**`internal/pkg/cookie/cookie.go`** (helper baru):

- `AccessFromRequest(r)`, `RefreshFromRequest(r)` — baca cookie dengan name &
  path konsisten.
- `SetAccess`, `SetRefresh` — tulis cookie dengan attributes wajib di atas.
- `Clear` — overwrite dengan MaxAge=-1 saat logout.

**`internal/app/middleware/auth.go`** — strict cookie-only:

```go
// SEBELUM
tok := cookie.AccessFromRequest(c.Request)
if tok == "" {
    raw := c.GetHeader(constant.HeaderAuthorization)
    if !strings.HasPrefix(raw, "Bearer ") {
        response.Error(c, apperror.Unauthorized(...))
        return
    }
    tok = strings.TrimPrefix(raw, "Bearer ")
}

// SESUDAH
tok := cookie.AccessFromRequest(c.Request)
if tok == "" {
    response.Error(c, apperror.Unauthorized(apperror.CodeAuthUnauthenticated))
    return
}
```

Middleware juga inject `JTI` + `Expires` ke ctxkey sehingga Logout bisa
blacklist tanpa parse header lagi.

**`internal/app/service/auth_service.go`:**

- `Login(ctx, req) (*model.User, TokenPair, error)` — return `TokenPair`
  internal (bukan DTO), controller yang convert ke cookie.
- `Refresh(ctx, refreshToken string) (*model.User, TokenPair, error)` —
  terima string langsung, bukan `request.Refresh` DTO.

**`internal/app/controller/auth_controller.go`:**

```go
// Login & Refresh: cookie set, body kosong
h.setAuthCookies(c, pair)
response.SuccessEmpty(c)   // → {meta, data: []}

// Refresh: HANYA dari cookie, tidak ada body fallback
rt := cookie.RefreshFromRequest(c.Request)
if rt == "" {
    response.Error(c, apperror.Unauthorized(apperror.CodeAuthUnauthenticated))
    return
}

// Logout: ambil JTI dari ctxkey (di-inject middleware), bukan parse header
jti := ctxkey.TokenJTI(ctx)
exp := ctxkey.TokenExpires(ctx)
```

**`internal/pkg/response/response.go`** — tambah helper:

```go
// SuccessEmpty writes {meta, data: []} for endpoints with no payload.
func SuccessEmpty(c *gin.Context) {
    c.JSON(http.StatusOK, envelope{
        Meta: buildMeta(c, nil),
        Data: []any{},
    })
}
```

**Hapus:**
- `internal/app/dto/response/auth_response.go` — `Tokens` struct sudah tidak
  perlu (bukan DTO API lagi).
- `request.Refresh` di `internal/app/dto/request/auth_request.go`.
- Spec: `bearerAuth` security scheme, `Tokens`/`TokensEnvelope`/`RefreshRequest`
  schemas. Tambah `EmptyEnvelope`.
- `Authorization` di `internal/pkg/constant/header.go` & CORS allow-list —
  tidak ada code yang baca header itu lagi.

### Hard rule

**Tidak ada `c.GetHeader("Authorization")` atau `Bearer ` di code app.**
Greppable check (CI bisa enforce):

```bash
grep -r "Authorization\|Bearer " internal/app/ internal/pkg/ | grep -v test
# → cuma policy comments yang muncul. Kalau ada code path baru, regresi.
```

### Verifikasi

```bash
# 1. Login → expect data: []
curl -s -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@local.com","password":"Admin@123"}'
# {"meta":{...},"data":[]}

# 2. Pakai cookie untuk admin endpoint → 200
curl -s -b cookies.txt http://localhost:8080/api/admin/settings/pricing
# {"meta":{...},"data":{"pricing":{...}}}

# 3. Pakai Bearer header tanpa cookie → MUST 401
TOKEN=$(grep access_token cookies.txt | awk '{print $7}')
curl -s -o /dev/null -w "%{http_code}\n" \
  -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/me
# 401     ← kalau muncul 200 di sini, bug regresi: middleware terima Bearer lagi

# 4. Refresh → cookie-only, body kosong
curl -s -b cookies.txt -c cookies2.txt -X POST http://localhost:8080/api/auth/refresh
# {"meta":{...},"data":[]}
```

### FE pattern (admin Svelte / customer Nuxt)

Kedua FE pakai pola yang sama persis:

```js
// Login
await fetch(`${API_BASE}/auth/login`, {
  method: 'POST',
  credentials: 'include',           // wajib — biar accept Set-Cookie
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ email, password })
});

// Setelah login: fetch user info
const res = await fetch(`${API_BASE}/me`, { credentials: 'include' });
const { data: { user } } = await res.json();
// user.role === 'admin' → redirect ke admin dashboard
// user.role === 'user'  → redirect ke customer home
```

`role` discriminator menentukan UI mana yang di-render, **bukan** endpoint
login terpisah. Backend tetap satu endpoint, satu auth mechanism.

### Pelajaran

**Pilih satu auth flow dan stick with it.** Dual-mode (cookie + bearer)
kelihatan flexible tapi maintenance cost-nya nyata. Kalau ada keperluan
mobile native di masa depan, evaluasi ulang — tapi jangan re-introduce Bearer
"just in case" tanpa real consumer.

`HttpOnly cookie + SameSite=Lax + Secure (prod)` sudah cukup proteksi untuk
admin dashboard + customer ecommerce yang full web.

---

## 8. Pre-launch migration policy: per-table file pattern

### Konteks

Selama project **belum production** (belum ada customer beneran di DB),
schema changes apa pun **di-consolidate ke file tabel yang relevan**, BUKAN
ditambah sebagai migrasi baru.

Background — kondisi awal boilerplate punya migrasi historical seperti:

- `20260430120000_drop_guest_role` — drop role yang sebenarnya tidak pernah
  ada di prod kita
- `20260501000000_user_role_fk` — add FK setelah init_schema
- `20260501010000_roles_timestamps` — add `created_at`/`updated_at` ke roles
- `20260502120000_create_settings` — add settings table
- `20260503000000_users_auth_provider` — add auth_provider columns

Sequence ini "benar" secara historis kalau prod sudah jalan dan data harus
dipertahankan, tapi **bikin onboarding baru ribet**: dev baru clone repo →
`make db-reset` → 7 migrasi historical jalan sequential → kebingungan baca
add-then-remove dance.

### Aturan

**File organization: pisah per-tabel.** Bukan satu `init_schema` besar.

```
20260424120000_create_function_set_updated_at.{up,down}.sql
20260424120010_create_table_roles.{up,down}.sql
20260424120020_create_table_users.{up,down}.sql
20260424120030_create_table_refresh_tokens.{up,down}.sql
20260424120040_create_table_audit_logs.{up,down}.sql
20260424120050_create_table_settings.{up,down}.sql
```

Tiap `create_table_X` file isinya self-contained: DDL + indexes + constraints + triggers + canonical seed (kalau ada). Mau ubah kolom → edit file tabel itu. Mau tambah tabel → bikin file baru.

**Sampai first prod deploy:**

| Action | Boleh? |
|---|---|
| Edit file `create_table_X.up.sql` untuk reflect final state | ✅ |
| Edit `create_table_X.down.sql` correspondingly | ✅ |
| Bikin file baru `create_table_NEW.{up,down}.sql` (untuk tabel baru) | ✅ |
| Bikin migrasi baru `2026..._add_phone_to_users.up.sql` | ❌ edit `create_table_users` langsung |
| Bikin migrasi cleanup `2026..._drop_X_column.up.sql` | ❌ edit file tabel langsung |
| Pindah DDL dari seeder ke `create_table_X` | ✅ |
| Jalankan `make db-reset` setelah edit | ✅ wajib |
| FK ordering: tabel yang ref-ed punya version lebih kecil | ✅ wajib (FK validity per step) |

**Setelah first prod deploy:**

- Init_schema **frozen** — never edit lagi.
- Schema changes wajib pakai migrasi additive baru di atas init.
- `down` migrations harus benar-benar reversible (atau marked one-way).
- Tambahkan changelog "switch to additive-only migrations" di
  `backend-user-story.md`.

### Kenapa pre-launch consolidate?

1. **Single source of truth:** dev baca `create_table_X.up.sql` langsung tahu
   skema akhir. Gak perlu mental-replay 7 migrasi.
2. **No add-then-remove:** kalau salah desain (mis. `dp_pct` yang kemudian
   di-drop), tinggal edit file tabel langsung. Gak ada noise di history.
3. **Onboarding cepat:** clone repo → `make db-reset` → langsung dev. Tidak
   tergantung urutan apply yang error-prone.
4. **Schema = migration, data = seeder** — DDL HANYA di migration. Seeder
   isinya hanya `INSERT`/`UPSERT` (lihat §3 di file ini).

### Konsekuensi praktis

- Saat dev tambah feature dengan schema baru: edit file tabel relevant, tambah
  `DROP` di down yang sesuai, lalu `make db-reset`.
- **JANGAN** apply via `make migrate-up` setelah edit file existing — golang-migrate
  akan complain karena version sudah dianggap applied. Gunakan `db-reset`.
- Test migrasi: `make migrate-down && make migrate-up` masih harus clean
  (round-trip), karena nanti pas prod, init_schema pun akan jadi migration
  pertama yang harus reversible.

### Cutoff signal

Project dianggap "post-launch" begitu:

- Customer pertama register & order → backup DB harus dijaga
- Domain `chasastore.com` live di public
- Atau owner tandai eksplisit di `backend-user-story.md` changelog

Setelah itu: switch ke additive-only. Update boilerplate-fixes ini dengan
catatan "freeze date".

### Pelajaran

Pre-launch boilerplate sering ditambahin "fix-up migrations" reflexively
karena pola migrate-up/down ngajarin "never edit history". Tapi prinsip itu
hanya berlaku **setelah ada data prod**. Selagi belum, edit langsung lebih
sehat — fewer files, clearer schema. Memory
`feedback_migration_consolidation.md` enforce ini di session future.

---

## 9. Cache-first reads — Redis dulu sebelum hit DB

### Konteks

Setiap fitur baru yang punya **read endpoint**, defaultnya **cek Redis dulu,
kalau kosong baru hit DB**. Pattern ini disebut **cache-aside**, dan sudah
diterapkan di `SettingsService` (lihat `internal/app/service/settings_service.go`).

Tujuan:
- **Latency** — hit Redis ~1ms vs DB query ~5-20ms (untuk indexed query)
- **DB load** — kurangi pressure ke Postgres untuk hot reads
- **Multi-instance consistency** — semua BE pod baca cache yang sama (saat
  kita scale horizontal nanti)

### Aturan

**Default ON (apply tanpa nanya)** — read endpoint dengan karakteristik:

- Read frequency tinggi (bisa > 1x/menit dari FE)
- Data relatif stabil (write rare dibanding read)
- Cache key bisa deterministik dari path/query

**Default OFF (skip cache, DB direct)** — endpoint dengan karakteristik:

| Kategori | Alasan skip |
|---|---|
| Mutation (POST/PATCH/DELETE) | Write paths — bukan tujuan cache |
| Payment status reads | Money — staleness bahaya |
| Audit log lists | Write-heavy, jarang di-read |
| List dengan many filter combos | Cache key explosion (`q × role × status × page`) |
| User-specific write-heavy data | Low hit ratio (order history) |
| Real-time data | Tujuan justru fresh |

### Pattern reference

```go
// READ — Redis-first
raw, err := s.redis.Get(ctx, cacheKey).Bytes()
if err == nil {
    var v T
    if json.Unmarshal(raw, &v) == nil { return &v, nil }
    // corrupt → fall through
} else if !errors.Is(err, redis.Nil) {
    // Redis down → fall through (service tetap jalan)
}

v, err := s.repo.Find(ctx, key)            // DB fallback
if err != nil { return nil, apperror.Internal(err) }

if data, _ := json.Marshal(v); data != nil {
    _ = s.redis.Set(ctx, cacheKey, data, ttl).Err()  // best-effort
}
return v, nil

// WRITE — invalidate-on-write
if err := s.repo.Update(ctx, ...); err != nil { return ... }
_ = s.redis.Del(ctx, cacheKey).Err()       // best-effort
```

### Properties wajib

1. **Stateless service struct** — TIDAK ADA `sync.Mutex`/map/cachedAt. Redis pegang state.
2. **DB fallback** — Redis miss/down → tetap respond (cache = optimization, bukan dependency).
3. **Best-effort cache writes** — `_ = redis.Set/Del` (ignore error).
4. **TTL safety net** — 60-300s, primary mechanism = invalidate-on-write.
5. **Synchronous invalidation** — DEL dalam request lifecycle yang sama dengan DB write. JANGAN async.
6. **Single key untuk data atomik** atau **per-entity key**. **JANGAN cache filter-combo lists** (cache key explosion).

### Quick checklist (4 pertanyaan sebelum tambah cache)

1. Read >> Write? (rasio ≥ 10:1)
2. Staleness toleran? (data 30s-5min telat masih OK secara bisnis)
3. Cache key deterministik? (dari path/id, BUKAN filter combo besar)
4. Multi-instance ke depan? (shared cache critical untuk konsistensi)

**≥ 3 jawaban "ya" → cache. < 3 → skip.**

### Concrete plan per fitur

| Fitur | Cache? | Key strategy |
|---|---|---|
| Settings/pricing | ✅ done | `settings:all` single key, invalidate on PATCH |
| Roles list | ✅ apply | `roles:all`, invalidate on Create/Update/Delete |
| User by ID | ✅ apply | `user:{id}` per-id, invalidate on update; TTL 5min |
| **Users list admin** | ❌ skip | Filter combos banyak |
| Products list public | ✅ apply | `products:list:{filter-hash}`, DEL pattern on scrape |
| Product by ID | ✅ apply | `product:{id}`, invalidate on scrape upsert |
| Orders list per user | ❌ skip | Write-heavy by same user |
| Order by ID | ✅ apply (light) | `order:{id}`, TTL 30s |
| **Payment status** | ❌ skip | Money — fresh wajib |
| Audit logs | ❌ skip | Write-heavy |

### Common pitfalls

- ❌ **Lupa invalidate** dari path mutation tak terduga (admin endpoint lain, scheduled job, webhook). **Fix:** tambah test integration yang verify cache cleared setelah mutation.
- ❌ **Async invalidate** (queue DEL Redis) — bikin window stale. **Fix:** synchronous dalam request lifecycle.
- ❌ **`KEYS pattern*`** untuk pattern delete — blocking di prod. **Fix:** maintain index set (`SADD products:cache_keys key`) lalu loop DEL, atau pakai SCAN.
- ❌ **`sync.Mutex` + map sebagai L1** plus Redis sebagai L2 — over-engineered untuk MVP. Stick to Redis-only sampai metrik bukti L1 dibutuhkan.

### Tools

`internal/pkg/cache/`:
- `cache.Cache` (di `cache.go`) — wrapper `SetJSON`/`GetJSON`/`Del`. Godoc top-of-file
  isinya pattern reference cache-aside lengkap, baca dulu sebelum bikin service baru.
- `keys.go` — tambah constant untuk tiap fitur baru.

### Pelajaran

Cache-first bukan optimisasi premature — ini convention konsistensi. Saat
multi-instance, in-memory cache jadi split-brain (`SettingsService` rev awal
ada masalah ini). Redis-only stateless service jadi default karena trade-off
extra ~1ms latency vs konsistensi cluster-wide adalah no-brainer untuk
business app.

Memory `feedback_redis_cache_first.md` enforce convention ini di session
future — saat ada PR fitur baru tanpa cache layer, agent harus push back.

---

## 10. Local Setup Checklist

Langkah-langkah dari clone-baru sampai server jalan di local.

```bash
# 1. Generate Paseto key (sekali, simpan di .env)
openssl rand -hex 32

# 2. Tempel ke .env (lihat §11):
#    PASETO_SYMMETRIC_KEY=<hasil>

# 3. Postgres + Redis up (compose file disertakan, lihat §12)
docker compose up -d

# 4. Pastikan database ada (kalau tidak pakai compose default DB)
createdb {{ .DBName }}   # atau via psql / GUI

# 5. Install CLI tools (migrate dengan -tags postgres, air untuk live reload)
make tools

# 6. Run dengan seed (lebih aman karena seeder upsert admin + roles)
make seed
# Output:
#   migrations applied {version: 20260424120040, dirty: false}
#   roles seeded {count: 2}
#   user admin@local upserted
#   seed done

# 7. Lanjut run server biasa
make run    # atau make dev untuk live-reload via air
```

`make seed` dan `make run` sama-sama trigger migration. Bedanya `seed` juga
upsert default data (admin + roles), `run` cuma migrate + listen.

Default admin (ganti setelah login pertama):

- email: `admin@local`
- password: `Admin@123`

---

## 11. Environment Variables

`.env.example` (di-generate oleh `chasago init`) berisi semua variabel.
Yang **wajib** di-set di `.env` lokal:

| Var | Contoh | Catatan |
|-----|--------|---------|
| `APP_ENV` | `development` / `production` | Drive `Secure` cookie flag (§7) |
| `PASETO_SYMMETRIC_KEY` | hex 64 char | `openssl rand -hex 32` |
| `DB_PASSWORD` | (sesuai compose) | jangan commit |
| `SMTP_*` | mailtrap / sendgrid | reset password butuh ini |
| `FRONTEND_URL` | `http://localhost:3000` | dipakai di reset-password link |
| `ALLOWED_ORIGINS` | `http://localhost:3000,...` | comma-separated, CORS |

Defaults yang biasanya tidak perlu di-override (lihat
`internal/platform/config/config.go` `setDefaults`):

- `APP_PORT=8080`, `DB_HOST=localhost`, `DB_PORT=5432`, `REDIS_ADDR=localhost:6379`
- `ACCESS_TOKEN_TTL=15m`, `REFRESH_TOKEN_TTL=168h` (7 hari)
- `RATE_LIMIT_RPS=10`, `AUTH_RATE_LIMIT_RPM=5`, `LOCKOUT_MAX_ATTEMPTS=5`

Production wajib explicit set `APP_ENV=production` — kalau tidak, cookie
`Secure=false` (HTTP plain) dan token bisa di-MITM.

---

## 12. Docker Compose Notes

`docker-compose.yml` (di-generate oleh `chasago init`) menyediakan Postgres +
Redis untuk dev lokal. Konvensi:

- **Port mapping** — Postgres 5432, Redis 6379. Kalau host udah occupied
  (`make run` error `connection refused`), edit compose port atau matikan
  service host.
- **Healthcheck** — service `app` punya `depends_on` ke postgres dengan
  condition `service_healthy`. Tidak ada race "DB belum siap → migrate fail".
- **Volume** — Postgres data persist di named volume; `make db-reset` cuma
  drop schema, BUKAN volume. Untuk wipe fisik: `docker compose down -v`.
- **JANGAN compose up service `app`** saat development — jalankan `make dev`
  di host supaya hot-reload via air. Compose hanya untuk dependency (DB +
  Redis).

---

## Changelog

| Tanggal | Event |
|---|---|
| 2026-04-30 | Tiga issue boilerplate ditemukan saat `make run` pertama. Fix: `fx.In` di `router.Deps`, `search_path=public` di DSN, `IF EXISTS` guard di migration `drop_guest_role`. |
| 2026-04-30 | Tambah catatan §5 (migrate CLI butuh `-tags postgres`, fix via `make tools`) dan §6 (Laravel `migrate:fresh` equivalent: `make db-reset`). |
| 2026-05-02 | Tambah §9: cache-first reads (Redis dulu sebelum DB). Default ON untuk read endpoints, exception list (mutations, payments, big-filter lists, audit, real-time). Stateless service struct (no L1 in-memory). Pattern reference: `SettingsService`. Memory `feedback_redis_cache_first.md`. Concrete plan per fitur ada di tabel — Settings ✅ done, Roles/User-by-ID/Products akan apply, Users-list-admin/Orders-list/Payments skip. |
| 2026-05-02 | Tambah §8: pre-launch migration policy. Awalnya consolidate ke single `init_schema.up.sql`, lalu di-refine ke **per-table file pattern**: `create_function_set_updated_at`, `create_table_roles`, `create_table_users`, `create_table_refresh_tokens`, `create_table_audit_logs`, `create_table_settings`. Tiap tabel = 1 file. Tambah kolom = edit file tabel langsung, jangan bikin `add_X_to_Y.up.sql`. Cutoff: first prod deploy. Memory `feedback_migration_consolidation.md`. |
| 2026-05-02 | Tambah §7: auth strict cookie-only. Hapus dual-mode (cookie + Bearer header) di middleware. Login/Refresh response body jadi `{meta, data: []}` (cookie sudah cukup). Single flow untuk admin + customer login: pakai endpoint yang sama, bedakan UI berdasarkan `user.role` dari `GET /api/me`. |
| 2026-05-03 | Port-back semua fix dari downstream ke template generator (`internal/template/files/`). Apply: §1 `fx.In` di Deps, §3 long-term (DDL `roles` ke migration, seeder insert-only), §7 strict cookie-only end-to-end (helper `internal/pkg/cookie/`, middleware tanpa Bearer fallback, `SuccessEmpty` helper, JTI/exp via ctxkey, hapus `Tokens` & `Refresh` DTO, hapus `Authorization` dari CORS allow-list & constant), §8 split init_schema jadi per-table files, §9 godoc cache-aside di `cache.go`. Re-order section 1-9 ke kronologis fisik, bungkus orphan code block jadi §10 Local Setup Checklist, tambah §11 Environment Variables & §12 Docker Compose Notes. Verifikasi: `chasago init → go build ./...` clean; `grep -r "Authorization\|Bearer "` di app code hanya match policy comments. |
