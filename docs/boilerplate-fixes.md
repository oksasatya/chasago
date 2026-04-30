# Boilerplate Fixes Log

Dokumentasi error yang ditemui saat pertama kali coba `make run` di project ini,
plus root cause + fix-nya. Disimpan supaya kalau ke depan ada onboarding atau
issue serupa, tinggal dibaca.

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

**Opsi B — di DSN (kalau tidak bisa atau tidak mau touch DB config):**

```go
// internal/platform/config/config.go
return fmt.Sprintf(
    "postgres://%s:%s@%s:%d/%s?sslmode=%s&search_path=public",
    d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
)
```

Pin `search_path=public` di DSN. Codebase saat ini **tidak** pakai opsi B —
ada catatan di komentar `DSN()` sebagai pengingat.

### Pelajaran

Kalau pakai `pgx` + golang-migrate `postgres` driver, **selalu** set
`search_path` eksplisit di DSN. Ini behaviour yang berbeda dari `lib/pq` yang
biasanya implicit fallback ke `public`.

---

## 3. Migration `drop_guest_role` — `relation "roles" does not exist`

### Gejala

```
ERROR  fx  OnStart hook failed
  error: migrate up: migration failed in line 0: ...DELETE FROM roles WHERE name = 'guest';
  (details: ERROR: relation "roles" does not exist (SQLSTATE 42P01))
```

Auto-migrate jalan, tapi migration `20260430120000_drop_guest_role.up.sql`
gagal di DB fresh karena referensi ke table `roles`.

### Root Cause

Boilerplate ini punya **inkonsistensi arsitektur**: table `users`,
`refresh_tokens`, `audit_logs` dibuat lewat **migration**
(`internal/database/migrations/20260424120000_init_schema.up.sql`), tapi table
`roles` (lookup table untuk role list) dibuat lewat **seeder**
(`internal/database/seeders/role_seeder.go`):

```go
const ddl = `CREATE TABLE IF NOT EXISTS roles (...)`
```

Hasilnya:

- Fresh DB tanpa `make seed` → server start → migrations run →
  `drop_guest_role` mencoba `DELETE FROM roles ...` → fail karena `roles`
  belum ada.
- Setelah `make seed` → `roles` ada, migrasi jalan normal.

Jadi urutan tergantung: kalau pertama kali run pakai `make run` (bukan
`make seed`), startup gagal.

### Fix (sementara)

Migrasi `drop_guest_role.up.sql` di-guard dengan `IF EXISTS` check supaya aman
di DB fresh:

```sql
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'roles'
    ) THEN
        DELETE FROM roles WHERE name = 'guest';
    END IF;
END $$;
```

Kalau `roles` tidak ada (fresh DB tanpa seed), DELETE di-skip — tidak ada
yang perlu dibersihkan. Schema constraint pada `users` tetap ditambahkan.

### Fix (long-term, recommendation)

Pindahkan pembuatan table `roles` dari seeder ke migration. Seeder seharusnya
hanya **insert data**, bukan create schema. Kalau dipindah, urutan startup jadi:

1. Migrations: bikin `users`, `refresh_tokens`, `audit_logs`, `roles`
2. (Opsional) seed: insert default rows ke `roles` + admin user

Belum dieksekusi karena bukan blocker untuk Sprint 1; cukup ditandai sebagai
techdebt.

### Pelajaran

**Schema = migration. Data = seeder.** Jangan campur. Kalau seeder bikin
DDL (`CREATE TABLE`), refactor ASAP — jadi sumber bug yang aneh kayak ini.

---

## 4. Migration Dirty State Recovery

### Kapan Terjadi

Kalau salah satu migration di atas gagal di tengah jalan, golang-migrate akan
menandai schema_migrations row dengan `dirty=true` untuk versi terakhir yang
dicoba. Server berikutnya start akan terus error:

```
ERROR  Dirty database version 20260430120000. Fix and force version.
```

### Cara Recovery

Pakai CLI golang-migrate (install via `brew install golang-migrate`):

```bash
# Force ke versi sebelum yang gagal (init_schema = 20260424120000)
migrate -database "postgres://postgres:postgres@localhost:5432/jastipchasastore_dev?sslmode=disable" \
        -path ./internal/database/migrations \
        force 20260424120000

# Lalu re-run migrate up
migrate -database "..." -path ./internal/database/migrations up
```

Atau langsung manipulasi `schema_migrations` table di Postgres:

```sql
-- Cek state
SELECT * FROM schema_migrations;

-- Reset dirty flag (hati-hati, hanya kalau yakin migrasi belum apply atau sudah di-apply manual)
UPDATE schema_migrations SET dirty = false;
```

Setelah itu `make run` (atau startup biasa) akan retry migration yang gagal.

---

## 5. Quick Setup Checklist (kalau onboarding baru)

```bash
# 1. Generate Paseto key (sekali, simpan di .env)
openssl rand -hex 32

# 2. Tempel ke jastipchasastore-be/.env:
#    PASETO_SYMMETRIC_KEY=<hasil>

# 3. Postgres + Redis up (docker compose biasanya)
docker compose up -d   # kalau ada docker-compose.yml

# 4. Bikin database (kalau belum ada)
createdb jastipchasastore_dev   # atau via psql / GUI

# 5. Run dengan seed (lebih aman karena seeder upsert admin + roles)
go run ./cmd/api seed
# Output:
#   migrations applied {version: 20260430120000, dirty: false}
#   roles seeded {count: 2}
#   user admin@local upserted
#   seed done

# 6. Lanjut run server biasa
go run ./cmd/api    # atau make run
```

`make seed` dan `make run` sama-sama trigger migration. Bedanya `seed` juga
upsert default data (admin + roles), `run` cuma migrate + listen.

---

## Changelog

| Tanggal | Event |
|---|---|
| 2026-04-30 | Tiga issue boilerplate ditemukan saat `make run` pertama. Fix: `fx.In` di `router.Deps`, `search_path=public` di DSN, `IF EXISTS` guard di migration `drop_guest_role`. |
