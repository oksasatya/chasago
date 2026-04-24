# chasago

CLI generator untuk boilerplate Go REST API — **Clean Architecture**, Gin,
uber-fx, sqlx + pgx, Paseto v4, Redis, golang-migrate (embedded), zap +
lumberjack, validator v10, Paseto blacklist & refresh-token rotation, idempotent
seeder, auto super-admin, CORS + security headers, audit log, soft delete, dan
email SMTP + i18n.

Jalankan `chasago init` di dalam folder project kosong — 1 command, project
siap `make dev`.

---

## Install

**Prasyarat:** Go 1.23+, `$GOBIN` sudah ada di `$PATH`.

```bash
go install github.com/oksasatya/chasago/cmd/chasago@latest
```

Cek terpasang:

```bash
chasago --version
# chasago dev (none, unknown)
```

> Kalau `chasago` tidak ditemukan setelah install, tambahkan `$(go env GOBIN)`
> atau `$(go env GOPATH)/bin` ke `$PATH` di `~/.zshrc` / `~/.bashrc`:
> ```bash
> export PATH="$(go env GOPATH)/bin:$PATH"
> ```

### Install dari source (development)

```bash
git clone https://github.com/oksasatya/chasago.git
cd chasago
go install ./cmd/chasago
```

### Uninstall

```bash
rm "$(command -v chasago)"
```

---

## Quick start — bikin project baru

```bash
# 1. buat folder kosong & masuk ke sana
mkdir pos-contoh && cd pos-contoh

# 2. jalankan chasago (prompt interaktif)
chasago init
```

Prompt akan menanyakan:

- **Go module path** — mis. `github.com/oksasatya/pos-contoh`
- **App name** — default dari nama folder
- **Database name** — default dari nama folder (underscore-ed)
- **Default timezone** — default `Asia/Jakarta`
- **Fitur auth** — centang: `register`, `login`, `forgot password`, `refresh token`
- **Enable email** — SMTP + reset-password flow

Selesai prompt, chasago akan:

1. Render seluruh template project ke folder saat ini
2. Jalanin `go mod tidy`
3. Inisialisasi `git init`
4. Tampilkan banner "Next steps"

### Non-interaktif (CI / scripting)

```bash
chasago init --yes \
  --module=github.com/oksasatya/pos-contoh \
  --app=pos-contoh \
  --db=pos_contoh \
  --timezone=Asia/Jakarta
```

Flag yang tersedia:

| Flag         | Default       | Keterangan                                  |
|--------------|---------------|---------------------------------------------|
| `--yes`      | `false`       | skip prompt interaktif, pakai default       |
| `--module`   | `github.com/your-org/<folder>` | override Go module path     |
| `--app`      | nama folder   | override app name                           |
| `--db`       | nama folder (underscored) | override database name          |
| `--timezone` | `Asia/Jakarta`| override default timezone                   |

---

## Setelah generate — setup project

```bash
# di dalam folder project yang baru di-generate
make init-env                    # copy .env.example → .env
# edit .env (DB_PASSWORD, SMTP_*, FRONTEND_URL, PASETO_SYMMETRIC_KEY, dll)

docker compose up -d postgres redis
make tools                       # install air (live-reload) + migrate CLI
make dev                         # server jalan di :8080 dgn auto-reload
```

**Login pertama:**

- email: `admin@local`
- password: `Admin@123`

Wajib ganti via `PATCH /api/me/password` atau edit const di
`internal/admin/ensure.go` sebelum deploy.

---

## Apa yang di-generate

Struktur lengkap (lihat `README.md` di dalam project yang di-generate untuk detail):

```
<project>/
├── cmd/api/main.go                # 13-line entrypoint (fx.New + invoke)
├── internal/
│   ├── model/              repository/      service/
│   ├── controller/         dto/{request,response}/
│   ├── mapper/             middleware/      router/
│   ├── module/             (11 fx modules)
│   ├── token/              apperror/        response/
│   ├── validator/          logger/          mailer/ + locales
│   ├── pagination/         database/        migrate/ + migrations/
│   ├── seeder/             constant/        ctxkey/
│   ├── cache/              audit/           admin/
│   └── config/
├── .env.example            .gitignore       .editorconfig
├── .air.toml               .golangci.yml
├── .github/workflows/ci.yml
├── Dockerfile              docker-compose.yml
├── Makefile                README.md
└── go.mod
```

### Endpoint default yang terpasang

```
POST  /api/auth/register
POST  /api/auth/login            → { access_token, refresh_token, expires_in }
POST  /api/auth/refresh          → rotate (reuse-detection → revoke family)
POST  /api/auth/logout  (auth)   → blacklist access token JTI di Redis
POST  /api/auth/forgot-password  → email reset link (pakai FRONTEND_URL)
POST  /api/auth/reset-password
GET   /api/me           (auth)
PATCH /api/me/password  (auth)
GET   /healthz
```

### Response envelope

Sukses:
```json
{
  "meta": { "request_id": "...", "timestamp": "..." },
  "data": { "user": { "id": "...", "email": "..." } }
}
```

Error (2 field: `code` = HTTP status int, `message` = semantic i18n key):
```json
{
  "meta": { "request_id": "...", "timestamp": "..." },
  "error": {
    "code": 400,
    "message": "VALIDATION_REQUIRED",
    "details": [
      { "field": "email", "message": "VALIDATION_REQUIRED" },
      { "field": "password", "message": "VALIDATION_MIN_LENGTH", "params": { "min": 8 } }
    ]
  }
}
```

---

## Aturan layering (enforced by convention)

| Layer        | Boleh berisi                                       | TIDAK boleh                        |
|--------------|----------------------------------------------------|------------------------------------|
| Controller   | bind → validate → call service → respond           | SQL, business logic                |
| Service      | semua business logic + transaksi                   | parsing HTTP, render response      |
| Repository   | **SEMUA SQL**                                      | business logic                     |
| Mapper       | konversi `model ↔ DTO request/response`            | logic bercabang                    |

---

## Yang sudah built-in (jangan bikin lagi)

- ✅ Request ID middleware (UUID v7, X-Request-ID header)
- ✅ Structured logging (zap JSON + lumberjack rotasi+gzip)
- ✅ CORS (dari `.env ALLOWED_ORIGINS`)
- ✅ Security headers (HSTS, CSP, XFO, XCTO, Referrer — hardcoded default)
- ✅ Recovery middleware (panic → 500 INTERNAL_ERROR + stacktrace log)
- ✅ Request timeout + max body size (dari `.env`)
- ✅ Rate limit global + **rate limit ketat per route `/api/auth/*`**
- ✅ Account lockout (5x gagal → lock 15 menit, Redis-backed)
- ✅ Password entropy validation (`wagslane/go-password-validator`, blacklist 10k)
- ✅ Paseto v4 local (symmetric) access token + refresh token rotation + reuse detection
- ✅ Access token blacklist via Redis (`bl:<jti>`)
- ✅ Soft delete (`deleted_at` kolom, base query `WHERE deleted_at IS NULL`)
- ✅ Trigger `set_updated_at` shared untuk seluruh tabel
- ✅ Audit log (tabel `audit_logs`, helper `audit.Log(ctx, action, ...)`)
- ✅ Idempotent seeder (UPSERT pattern)
- ✅ Auto-migrate + auto-seed saat `make dev` (toggle via env)
- ✅ Super-admin auto-create pada first boot (hardcoded `admin@local` / `Admin@123`)
- ✅ Graceful shutdown (fx lifecycle, HTTP drain 10s, DB/Redis close)

---

## Troubleshooting

**`chasago: command not found`** setelah `go install`
→ Tambah `$(go env GOBIN)` atau `$(go env GOPATH)/bin` ke `$PATH`. Ada detail
   di bagian [Install](#install).

**`go mod tidy` gagal saat generate**
→ Cek koneksi internet + `GOPROXY`. Kalau di-skip, boilerplate tetap jalan —
   `go.mod` masih valid, tinggal jalanin `go mod tidy` manual di folder project.

**Generated project: `database "xxx" does not exist`**
→ `docker compose up -d postgres redis` belum dijalankan, atau nama DB di
   `.env` beda dari yang dibuat container. Cek `DB_NAME` match dengan
   `docker compose logs postgres`.

**Lupa ganti password admin default**
→ Edit const di `internal/admin/ensure.go` atau ganti via endpoint
   `PATCH /api/me/password`. Server log `WARN: default admin password in use`
   saat default password masih aktif.

**Mau ulang dari awal (DB kotor)**
→ `make db-reset` di folder project → drop schema → fresh migrate → seed.

---

## Development chasago sendiri

```bash
git clone https://github.com/oksasatya/chasago.git
cd chasago
go build ./...
go test ./...

# manual test generate ke /tmp
go build -o /tmp/chasago-bin ./cmd/chasago
mkdir /tmp/test-proj && cd /tmp/test-proj
/tmp/chasago-bin init --yes --module=example.com/test --app=test --db=test_db
cd /tmp/test-proj && go build ./...
```

Struktur repo:

```
chasago/
├── cmd/chasago/main.go                  # CLI entrypoint (cobra)
├── internal/
│   ├── cli/                             # cobra commands + huh prompts
│   ├── template/                        # embed + text/template renderer
│   │   └── files/                       # template project (di-embed ke binary)
│   └── version/
└── go.mod
```

File template di `internal/template/files/` — suffix `.gotmpl` diproses oleh
`text/template` saat generate (module path, app name, dll di-inject).
File tanpa suffix (mis. email HTML templates, migrations SQL) di-copy apa adanya.

---

## Lisensi

MIT.
