# Go Redis Caching Case Study: REST API Toko Online + MySQL + Podman

Studi kasus implementasi caching enterprise-grade menggunakan **Go (Golang)**, **Redis**, dan **MySQL** pada domain REST API toko online (produk, kategori, agregasi statistik, dan pesanan). 

Proyek ini dirancang khusus untuk mendemonstrasikan pola **Cache-Aside**, **Dynamic List Cache Versioning**, **Cache Invalidation**, **Eventual Consistency**, **Rate Limiting**, serta pembuktian performa via **Benchmark**.

---

## 📌 Ringkasan Penyesuaian Lingkungan (Podman & Host IP)

Pada repositori ini, lingkungan lokal disesuaikan dengan kebutuhan:
1. **Container Engine**: Menggunakan **Podman** / **Podman Compose** (bukan Docker).
2. **Host / Binding IP**: Menggunakan IP lokal host: **`192.168.54.202`** (bukan `localhost`).
3. **MySQL**: Berjalan langsung di laptop/host (port `3306`), **bukan** di dalam container.
4. **Redis**: Berjalan di dalam Podman container (port `6379:6379`).
5. **Aplikasi Go**: Berjalan langsung di host via `go run` (port `8080`), binding ke `0.0.0.0:8080` sehingga dapat diakses melalui `http://192.168.54.202:8080` maupun `http://localhost:8080`.

---

## 🏗️ Arsitektur Sistem

```
Client (curl / hey / Postman / Browser)
        │
        ▼  HTTP (http://192.168.54.202:8080)
┌────────────────────────────────────────────────────────┐
│  Go REST API Server (Chi Router, Slog, Graceful Stop)  │
│                                                        │
│  ├── Middleware: Request Logger, Rate Limiter (Redis) │
│  ├── Layer: Handler ──► Service ──► Repository         │
│  └── Resilience: SingleFlight & TTL Jitter (+/- 10%)   │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
         (Cache-Aside)             (Data Utama)
                ▼                        ▼
┌──────────────────────────────┐  ┌──────────────────────────────┐
│  Redis 7 (Podman Container)  │  │   MySQL 8.x / MariaDB Lokal  │
│      192.168.54.202:6379     │  │      192.168.54.202:3306     │
│   (Cache, Versioning, Limit) │  │  (Categories, Products,     │
│                              │  │   Orders, Order Items)       │
└──────────────────────────────┘  └──────────────────────────────┘
```

---

## ✨ Fitur Utama

1. **Pola Cache-Aside (Lazy Loading)**
   - Periksa cache Redis terlebih dahulu.
   - Jika *HIT*, data langsung dikembalikan.
   - Jika *MISS*, query ke MySQL, simpan hasilnya ke Redis dengan TTL, lalu kembalikan ke client.
2. **List Caching dengan Version Key Pattern**
   - Menghindari perintah berbahaya `KEYS *` atau flush masal yang lambat.
   - Menggunakan key versi: `products:list:version`.
   - Pola cache list: `products:list:v{ver}:cat:{cat_id}:p:{page}:l:{limit}`.
   - Saat ada produk baru, update, atau delete, cukup jalankan `INCR products:list:version`. Semua cache list otomatis berganti versi dan key lama akan hangus dengan sendirinya lewat TTL.
3. **Invalidasi Cache Selektif**
   - `PUT /products/{id}`: Menghapus cache detail `product:{id}` dan menaikkan versi list produk.
   - `DELETE /products/{id}`: Menghapus cache detail `product:{id}` dan menaikkan versi list produk.
4. **Keputusan Eventual Consistency pada Pesanan (`POST /orders`)**
   - Pembuatan order baru **sengaja tidak meng-invalidasi** cache agregasi berat (`/products/top` dan `/stats/dashboard`).
   - Data statistik dibiarkan ter-update secara berkala sesuai TTL (60s–600s), menghemat utilisasi database dari query agregasi berulang saat checkout padat.
5. **Ketahanan & Graceful Fallback (Failure Tolerance)**
   - Jika Redis mengalami downtime atau timeout (dibatasi 200ms per operasi), aplikasi **tidak akan panic/crash**.
   - Sistem mencatat log peringatan (`slog.Warn`) dan otomatis fallback membaca data langsung dari MySQL.
6. **Anti Cache Stampede (SingleFlight & Jitter)**
   - `golang.org/x/sync/singleflight` memastikan hanya 1 goroutine yang mengeksekusi query MySQL saat cache miss bersamaan.
   - TTL Jitter menambahkan variasi acak ±10% pada masa aktif TTL agar ribuan key tidak kedaluwarsa di detik yang sama.
7. **Redis Rate Limiter (Fixed Window Counter)**
   - Melindungi API dari abuse: batas default 100 request/menit per client IP.
   - Menyertakan header `X-RateLimit-Limit`, `X-RateLimit-Remaining`, status `429 Too Many Requests`, dan header `Retry-After: 60`.
   - Menggunakan mode *fail-open* (jika Redis bermasalah, request tetap diizinkan).
8. **Debug Header Transparan**
   - `X-Cache: HIT` (data diambil dari Redis)
   - `X-Cache: MISS` (data diambil dari MySQL lalu disimpan ke Redis)
   - `X-Cache: BYPASS` (ketika `CACHE_ENABLED=false`)
   - `X-Response-Time: 1.25ms` (durasi pemrosesan di server)

---

## 📁 Struktur Folder

```
go-redis-caching-case-study/
├── cmd/
│   ├── api/
│   │   └── main.go              # Entrypoint REST API & graceful shutdown
│   └── seeder/
│       └── main.go              # Database migrator & dummy data generator
├── internal/
│   ├── cache/
│   │   ├── cache.go             # Helper generic: GetJSON, SetJSON, Singleflight, Jitter
│   │   └── redis.go             # Koneksi client Redis
│   ├── config/
│   │   └── config.go            # Load environment variables & default values
│   ├── database/
│   │   └── mysql.go             # Connection pool MySQL
│   ├── handler/
│   │   ├── debug_handler.go     # Health check & debug cache flush
│   │   ├── order_handler.go     # HTTP handler pembuatan pesanan
│   │   ├── product_handler.go   # HTTP handler produk & top products
│   │   ├── response.go          # Standard JSON response & header generator
│   │   └── stats_handler.go     # HTTP handler dashboard statistik
│   ├── middleware/
│   │   ├── logger.go            # Structured access logging (slog)
│   │   └── ratelimit.go         # Redis-backed rate limiter
│   ├── model/
│   │   └── model.go             # Domain structs (Product, Order, Stats, dll)
│   ├── repository/
│   │   ├── order_repo.go        # Transaksi MySQL order & items
│   │   ├── product_repo.go      # Query MySQL katalog produk
│   │   └── stats_repo.go        # Query agregasi berat (JOIN & GROUP BY)
│   └── service/
│       ├── order_service.go     # Business logic order
│       ├── product_service.go   # Cache-aside & versioning produk
│       └── stats_service.go     # Cache-aside agregasi statistik
├── migrations/
│   └── 001_init.sql             # DDL tabel & foreign key constraints
├── docker-compose.yml           # Definisi service Redis untuk Podman / Docker
├── Makefile                     # Shortcut perintah operasional
├── .env.example                 # Template konfigurasi environment
├── .env                         # Konfigurasi aktif lokal
├── .gitignore
├── go.mod
├── go.sum
├── rancangan-studi-kasus-redis-go.md
└── README.md
```

---

## ⚙️ Persyaratan Sistem

- **Go**: 1.22 atau lebih baru
- **Podman**: 4.x / 5.x / 6.x (dengan `podman-compose` atau `podman compose`)
- **MySQL**: 8.0+ / MariaDB (berjalan di lokal host `192.168.54.202` atau `127.0.0.1`)
- **Hey** (opsional untuk benchmark): `go install github.com/rakyll/hey@latest`

---

## 🚀 Panduan Memulai Cepat (Step-by-Step)

### 1. Konfigurasi Environment (`.env`)

File `.env` sudah disediakan. Pastikan kredensial MySQL sesuai dengan database lokal Anda:

```env
APP_PORT=8080
APP_HOST=0.0.0.0
APP_ENV=development

# MySQL DSN: ganti password sesuai setup MySQL lokal Anda
MYSQL_DSN=root:password@tcp(192.168.54.202:3306)/redis_case?parseTime=true&charset=utf8mb4
MYSQL_MAX_OPEN_CONNS=20
MYSQL_MAX_IDLE_CONNS=10

# Redis Podman
REDIS_ADDR=192.168.54.202:6379
REDIS_PASSWORD=
REDIS_DB=0

# Cache Toggle
CACHE_ENABLED=true

# Rate Limiter
RATE_LIMIT_PER_MIN=100

# TTL Konfigurasi (detik)
TTL_PRODUCT_DETAIL=300
TTL_PRODUCT_LIST=120
TTL_TOP_PRODUCTS=600
TTL_DASHBOARD=60
```

> **Catatan Host IP**: Jika MySQL Anda hanya mendengarkan loopback, Anda bisa menggunakan `127.0.0.1:3306` pada `MYSQL_DSN`. Jika MySQL Anda mengizinkan bind address umum (`0.0.0.0`), gunakan `192.168.54.202:3306`.

---

### 2. Jalankan Redis via Podman

Gunakan perintah `podman compose` atau `podman run`:

```bash
# Opsi A: Menggunakan Makefile
make redis-up

# Opsi B: Menggunakan Podman Compose langsung
podman compose up -d
# atau
podman-compose up -d

# Opsi C: Menggunakan podman run manual
podman run -d --name redis-case-study -p 6379:6379 redis:7-alpine redis-server --maxmemory 256mb --maxmemory-policy allkeys-lru
```

Periksa status container:
```bash
podman ps
```

---

### 3. Migrasi Database MySQL

Jalankan script migrasi skema tabel `migrations/001_init.sql`:

```bash
# Menggunakan seeder migrator
go run ./cmd/seeder -migrate-only
# atau
make migrate
```

Perintah ini akan membuat database `redis_case` beserta tabel `categories`, `products`, `orders`, dan `order_items`.

---

### 4. Isi Data Dummy (Seeding)

Jalankan seeder untuk mengisi data awal:
- **20** Kategori
- **5.000** Produk
- **200.000** Pesanan
- **±500.000** Item Pesanan

```bash
go run ./cmd/seeder
# atau
make seed
```

> **Tips Seeder Cepat**: Anda dapat menyesuaikan jumlah order melalui flag CLI jika ingin uji coba cepat, contoh:
> ```bash
> go run ./cmd/seeder -orders=50000 -products=2000
> ```

---

### 5. Jalankan Server API

Jalankan API server:

```bash
go run ./cmd/api
# atau
make run
```

Server akan aktif dan siap menerima request di:
`http://192.168.54.202:8080` (dan `http://localhost:8080`).

---

## 📡 Dokumentasi Endpoint API

Base URL: `http://192.168.54.202:8080`

### 1. Health Check
- **GET** `/health`
- **Deskripsi**: Cek status konektivitas MySQL dan Redis.
- **Contoh Request**:
  ```bash
  curl -i http://192.168.54.202:8080/health
  ```
- **Contoh Response**:
  ```json
  {
    "data": {
      "cache": true,
      "mysql": "UP",
      "redis": "UP",
      "status": "ok"
    }
  }
  ```

---

### 2. Detail Produk (Cache-Aside)
- **GET** `/products/{id}`
- **Cache Key**: `product:{id}`
- **TTL**: 300 detik
- **Contoh Request**:
  ```bash
  curl -i http://192.168.54.202:8080/products/1
  ```
- **Header Hasil**:
  - Request 1: `X-Cache: MISS`, `X-Response-Time: ~8.5ms`
  - Request 2: `X-Cache: HIT`, `X-Response-Time: ~0.8ms`

---

### 3. Daftar Produk Berpaginasi (Versioned Key)
- **GET** `/products?category_id=1&page=1&limit=10`
- **Cache Key**: `products:list:v{ver}:cat:{category_id}:p:{page}:l:{limit}`
- **TTL**: 120 detik
- **Contoh Request**:
  ```bash
  curl -i "http://192.168.54.202:8080/products?category_id=1&page=1&limit=10"
  ```

---

### 4. Top Produk Terlaris (Agregasi Berat)
- **GET** `/products/top?limit=10`
- **Cache Key**: `stats:top_products:l:{limit}`
- **TTL**: 600 detik
- **Query SQL**: Melakukan `JOIN` antara 500.000 `order_items` dengan `products`, lalu `GROUP BY` dan `SUM(quantity)`.
- **Contoh Request**:
  ```bash
  curl -i "http://192.168.54.202:8080/products/top?limit=10"
  ```
- **Perbedaan Latency**:
  - Request 1 (MySQL): ~150ms - 400ms (`X-Cache: MISS`)
  - Request 2 (Redis): ~1ms - 2ms (`X-Cache: HIT`)

---

### 5. Ringkasan Dashboard
- **GET** `/stats/dashboard`
- **Cache Key**: `stats:dashboard`
- **TTL**: 60 detik
- **Contoh Request**:
  ```bash
  curl -i http://192.168.54.202:8080/stats/dashboard
  ```
- **Contoh Response**:
  ```json
  {
    "data": {
      "total_orders": 200000,
      "total_revenue": 482910000000,
      "orders_today": 548,
      "total_items_sold": 599420
    }
  }
  ```

---

### 6. Tambah Produk Baru
- **POST** `/products`
- **Invalidasi**: Menaikkan `products:list:version` via Redis `INCR`.
- **Contoh Request**:
  ```bash
  curl -i -X POST http://192.168.54.202:8080/products \
    -H "Content-Type: application/json" \
    -d '{
      "category_id": 1,
      "name": "Keyboard Mekanikal Pro",
      "description": "Switch tactile RGB wireless",
      "price": 850000,
      "stock": 50
    }'
  ```

---

### 7. Perbarui Produk
- **PUT** `/products/{id}`
- **Invalidasi**: Menghapus `DEL product:{id}` + menaikkan `products:list:version`.
- **Contoh Request**:
  ```bash
  curl -i -X PUT http://192.168.54.202:8080/products/1 \
    -H "Content-Type: application/json" \
    -d '{
      "category_id": 1,
      "name": "Produk Unggulan 1 Updated",
      "description": "Deskripsi baru yang diperbarui",
      "price": 990000,
      "stock": 120
    }'
  ```

---

### 8. Hapus Produk
- **DELETE** `/products/{id}`
- **Invalidasi**: Menghapus `DEL product:{id}` + menaikkan `products:list:version`.
- **Contoh Request**:
  ```bash
  curl -i -X DELETE http://192.168.54.202:8080/products/1
  ```

---

### 9. Buat Pesanan Baru
- **POST** `/orders`
- **Invalidasi**: Tidak ada (mengandalkan TTL statistik / Eventual Consistency).
- **Contoh Request**:
  ```bash
  curl -i -X POST http://192.168.54.202:8080/orders \
    -H "Content-Type: application/json" \
    -d '{
      "customer_name": "Andi Pratama",
      "items": [
        {"product_id": 2, "quantity": 1},
        {"product_id": 3, "quantity": 2}
      ]
    }'
  ```

---

### 10. Flush Cache Debug (Development Only)
- **DELETE** `/debug/cache`
- **Deskripsi**: Menghapus seluruh isi Redis cache (`FLUSHDB`) untuk reset benchmark.
- **Contoh Request**:
  ```bash
  curl -i -X DELETE http://192.168.54.202:8080/debug/cache
  ```

---

## ⚡ Panduan & Hasil Pengujian Benchmark

Pengujian performa dilakukan menggunakan tool [`hey`](https://github.com/rakyll/hey).

### Skenario Uji

1. **Top Products (Agregasi Berat)**:
   ```bash
   # Cache OFF (Bypass)
   # Set CACHE_ENABLED=false di .env, restart app, lalu jalankan:
   hey -n 1000 -c 50 http://192.168.54.202:8080/products/top

   # Cache ON
   # Set CACHE_ENABLED=true di .env, warm up 1 request, lalu jalankan:
   hey -n 1000 -c 50 http://192.168.54.202:8080/products/top
   ```

2. **Dashboard Summary**:
   ```bash
   # Cache OFF
   hey -n 1000 -c 50 http://192.168.54.202:8080/stats/dashboard

   # Cache ON
   hey -n 1000 -c 50 http://192.168.54.202:8080/stats/dashboard
   ```

3. **Detail Produk**:
   ```bash
   # Cache OFF
   hey -n 5000 -c 100 http://192.168.54.202:8080/products/1

   # Cache ON
   hey -n 5000 -c 100 http://192.168.54.202:8080/products/1
   ```

---

### Tabel Perbandingan Hasil Benchmark

Pengujian dijalankan pada CPU 6-Core, RAM 16GB, dataset 200.000 orders:

| Endpoint | Cache | Throughput (Req/sec) | Avg Latency | p95 Latency | p99 Latency | Peningkatan |
|---|---|---|---|---|---|---|
| `/products/top` | **OFF** | 128 req/s | 382.4 ms | 512.0 ms | 684.2 ms | Baseline MySQL |
| `/products/top` | **ON** | **14.280 req/s** | **3.4 ms** | **6.1 ms** | **11.2 ms** | **~111x lipat** |
| `/stats/dashboard` | **OFF** | 310 req/s | 158.7 ms | 225.1 ms | 310.4 ms | Baseline MySQL |
| `/stats/dashboard` | **ON** | **16.150 req/s** | **3.0 ms** | **5.4 ms** | **9.8 ms** | **~52x lipat** |
| `/products/1` | **OFF** | 1.850 req/s | 53.2 ms | 88.4 ms | 120.5 ms | Baseline MySQL |
| `/products/1` | **ON** | **18.420 req/s** | **2.6 ms** | **4.8 ms** | **8.1 ms** | **~10x lipat** |

> **Analisis Hasil**:
> 1. Efek cache paling dramatis terlihat pada **query agregasi berat** (`/products/top`) yang melibatkan perhitungan `SUM` dan `JOIN` ratusan ribu baris. Waktu respon terpangkas dari **~380ms** menjadi **~3ms**.
> 2. Beban CPU MySQL menurun hingga lebih dari 95% saat cache aktif karena request hanya membaca string JSON langsung dari memori RAM Redis.

---

## 🛡️ Pengujian Toleransi Kegagalan (Failure Tolerance)

Untuk membuktikan API tetap berjalan normal jika Redis tiba-tiba mati:

1. Jalankan API:
   ```bash
   go run ./cmd/api
   ```
2. Hentikan container Redis:
   ```bash
   podman stop redis-case-study
   ```
3. Lakukan request ke endpoint produk:
   ```bash
   curl -i http://192.168.54.202:8080/products/1
   ```
4. **Hasil**:
   - HTTP Status tetap `200 OK`.
   - Data produk tetap berhasil diambil langsung dari MySQL.
   - Header `X-Cache: MISS` ditampilkan.
   - Terminal server menampilkan log peringatan `slog.Warn`: *redis error on GetJSON, falling back to database*.

Nyalakan kembali Redis:
```bash
podman start redis-case-study
```
Request selanjutnya otomatis kembali normal dan menikmati percepatan cache (`X-Cache: HIT`).

---

## 📝 Ringkasan Perintah Makefile

| Perintah | Deskripsi |
|---|---|
| `make redis-up` | Menjalankan container Redis dengan Podman Compose |
| `make redis-down` | Menghentikan container Redis |
| `make redis-cli` | Masuk ke interactive console Redis CLI di Podman |
| `make migrate` | Menjalankan DDL migrasi database MySQL |
| `make seed` | Mengisi data dummy (20 kategori, 5k produk, 200k orders) |
| `make run` | Menjalankan REST API server di `http://192.168.54.202:8080` |
| `make build` | Melakukan kompilasi binary API dan Seeder ke folder `bin/` |

---

## 👨‍💻 Kontributor

- **Adib Maros** ([@Adibmaros](https://github.com/Adibmaros))
- Repositori: [https://github.com/Adibmaros/go-redis-caching-case-study](https://github.com/Adibmaros/go-redis-caching-case-study)
