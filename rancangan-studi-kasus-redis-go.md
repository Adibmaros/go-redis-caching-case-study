# Rancangan Studi Kasus: Redis Caching pada REST API Go + MySQL

> Dokumen ini adalah spesifikasi lengkap untuk di-generate oleh AI di IDE.
> Baca seluruh dokumen, lalu implementasikan **bertahap sesuai urutan di bagian "Tahapan Implementasi"**.
> Setiap tahap harus bisa dijalankan dan dites sebelum lanjut ke tahap berikutnya.

---

## 1. Tujuan

Membuat proyek belajar untuk memahami **cara kerja Redis sebagai cache** di aplikasi nyata, dijalankan sepenuhnya di laptop lokal.

Yang ingin dipelajari:

1. Pola **cache-aside** (cek cache → miss → query DB → simpan ke cache).
2. Menentukan **data apa yang layak di-cache** dan **TTL**-nya.
3. **Invalidasi cache** saat data berubah.
4. **Membuktikan dampak cache** lewat benchmark (sebelum vs sesudah).
5. Bonus: pemakaian Redis di luar caching (**rate limiter**).

Domain: **toko online sederhana** (produk, kategori, pesanan).

---

## 2. Arsitektur & Lingkungan

```
Client (curl / hey / Postman)
        │
        ▼
Go REST API  (jalan di laptop via `go run`, bind 0.0.0.0:8080 / http://192.168.54.202:8080)
        │
        ├──► Redis  (Podman container, 192.168.54.202:6379)
        └──► MySQL  (instalasi lokal laptop, 192.168.54.202:3306 atau 127.0.0.1:3306)
```

- **Redis**: berjalan di Podman container (hanya Redis yang di-container-kan).
- **MySQL**: sudah terinstall di laptop (lokal), **bukan** di container.
- **App Go**: dijalankan langsung dengan `go run`, bukan di container. Koneksi menggunakan IP lokal host `192.168.54.202` (atau fallback `127.0.0.1`).
- Redis **hanya** menyimpan cache dan data sementara. **Sumber data utama tetap MySQL.**

---

## 3. Tech Stack

| Komponen | Pilihan |
|---|---|
| Bahasa | Go 1.22+ |
| HTTP router | `github.com/go-chi/chi/v5` |
| MySQL driver | `github.com/go-sql-driver/mysql` + `database/sql` (tanpa ORM) |
| Redis client | `github.com/redis/go-redis/v9` |
| Config | environment variable (`github.com/joho/godotenv` untuk load `.env`) |
| Logging | `log/slog` (standard library) |
| Validasi input | manual sederhana atau `github.com/go-playground/validator/v10` |
| Benchmark tool | `hey` (eksternal, dijalankan manual) |

Hindari dependency tambahan di luar daftar ini kecuali sangat diperlukan.

---

## 4. Struktur Folder

```
redis-case-study/
├── cmd/
│   ├── api/
│   │   └── main.go              # entrypoint server
│   └── seeder/
│       └── main.go              # entrypoint seeder data dummy
├── internal/
│   ├── config/
│   │   └── config.go            # load env ke struct Config
│   ├── database/
│   │   └── mysql.go             # koneksi MySQL (connection pool)
│   ├── cache/
│   │   ├── redis.go             # koneksi Redis
│   │   └── cache.go             # helper generic: GetJSON, SetJSON, Delete, dll
│   ├── model/
│   │   └── model.go             # struct Product, Category, Order, dll
│   ├── repository/
│   │   ├── product_repo.go      # query MySQL untuk produk
│   │   ├── order_repo.go        # query MySQL untuk pesanan
│   │   └── stats_repo.go        # query agregasi (top products, dashboard)
│   ├── service/
│   │   ├── product_service.go   # logika bisnis + cache-aside produk
│   │   ├── order_service.go
│   │   └── stats_service.go     # cache-aside untuk agregasi
│   ├── handler/
│   │   ├── product_handler.go
│   │   ├── order_handler.go
│   │   ├── stats_handler.go
│   │   └── response.go          # helper JSON response
│   └── middleware/
│       ├── logger.go            # log request + durasi
│       └── ratelimit.go         # (tahap 5) rate limiter berbasis Redis
├── migrations/
│   └── 001_init.sql             # DDL tabel + index
├── docker-compose.yml           # hanya service Redis
├── .env.example
├── Makefile
├── go.mod
└── README.md
```

Arsitektur berlapis: **handler → service → repository**. Logika cache berada di **service layer**, bukan di handler maupun repository.

---

## 5. Konfigurasi

### `.env.example`

```env
APP_PORT=8080
APP_HOST=0.0.0.0
APP_ENV=development

# MySQL DSN (gunakan 192.168.54.202 atau 127.0.0.1 sesuai setup lokal)
MYSQL_DSN=root:password@tcp(192.168.54.202:3306)/redis_case?parseTime=true&charset=utf8mb4
MYSQL_MAX_OPEN_CONNS=20
MYSQL_MAX_IDLE_CONNS=10

# Redis address (Podman exposed port pada IP host atau localhost)
REDIS_ADDR=192.168.54.202:6379
REDIS_PASSWORD=
REDIS_DB=0

# Toggle untuk benchmark: false = semua request langsung ke MySQL
CACHE_ENABLED=true

# Rate Limiter (req per menit per IP)
RATE_LIMIT_PER_MIN=100

# TTL (detik)
TTL_PRODUCT_DETAIL=300
TTL_PRODUCT_LIST=120
TTL_TOP_PRODUCTS=600
TTL_DASHBOARD=60
```

### `docker-compose.yml` (Kompatibel dengan Podman Compose)

```yaml
services:
  redis:
    image: redis:7-alpine
    container_name: redis-case-study
    ports:
      - "6379:6379"
    command: >
      redis-server
      --maxmemory 256mb
      --maxmemory-policy allkeys-lru
    restart: unless-stopped
```

### `Makefile` (target minimal untuk Podman / Docker)

```makefile
make redis-up      # podman compose up -d (atau podman-compose up -d)
make redis-down    # podman compose down (atau podman-compose down)
make migrate       # jalankan migrations/001_init.sql ke MySQL
make seed          # go run ./cmd/seeder
make run           # go run ./cmd/api
make redis-cli     # podman exec -it redis-case-study redis-cli
```

---

## 6. Skema Database (MySQL)

Database: `redis_case`. Engine: InnoDB, charset `utf8mb4`.

```sql
CREATE DATABASE IF NOT EXISTS redis_case CHARACTER SET utf8mb4;
USE redis_case;

CREATE TABLE categories (
  id         INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(100) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE products (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  category_id INT UNSIGNED NOT NULL,
  name        VARCHAR(200) NOT NULL,
  description TEXT,
  price       DECIMAL(12,2) NOT NULL,
  stock       INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (category_id) REFERENCES categories(id),
  INDEX idx_products_category (category_id)
);

CREATE TABLE orders (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  customer_name VARCHAR(100) NOT NULL,
  total_amount DECIMAL(14,2) NOT NULL,
  status       ENUM('pending','paid','shipped','cancelled') NOT NULL DEFAULT 'paid',
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_orders_created (created_at)
);

CREATE TABLE order_items (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  order_id   BIGINT UNSIGNED NOT NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  quantity   INT NOT NULL,
  unit_price DECIMAL(12,2) NOT NULL,
  FOREIGN KEY (order_id) REFERENCES orders(id),
  FOREIGN KEY (product_id) REFERENCES products(id)
);
```

> Catatan: `order_items` sengaja **tidak diberi index tambahan selain FK default** agar query agregasi "top products" cukup berat dan efek cache terlihat jelas saat benchmark.

---

## 7. Seeder Data Dummy

Program terpisah di `cmd/seeder/main.go`. Tujuan: membuat data cukup banyak agar MySQL benar-benar terasa lambat tanpa cache.

| Tabel | Jumlah |
|---|---|
| `categories` | 20 |
| `products` | 5.000 |
| `orders` | 200.000 |
| `order_items` | ±500.000 (1–5 item per order, produk acak) |

Persyaratan seeder:

- Gunakan **batch insert** (misal 1.000 baris per statement) di dalam transaksi, bukan insert satu per satu.
- Tampilkan progress di terminal.
- Data acak tapi masuk akal (nama produk "Produk 1234", harga 10.000–5.000.000, `created_at` tersebar 1 tahun terakhir).
- Aman dijalankan ulang: truncate tabel dulu (dengan `SET FOREIGN_KEY_CHECKS=0`) atau cek apakah data sudah ada.

---

## 8. Spesifikasi Endpoint

Base URL: `http://192.168.54.202:8080` (atau `http://localhost:8080`). Semua response berformat JSON.

### 8.1 Endpoint baca (di-cache)

| Method | Path | Deskripsi | Cache Key | TTL (env) |
|---|---|---|---|---|
| GET | `/products/{id}` | Detail satu produk | `product:{id}` | `TTL_PRODUCT_DETAIL` |
| GET | `/products?category_id=&page=&limit=` | List produk per kategori dengan paginasi | `products:list:v{ver}:cat:{category_id}:p:{page}:l:{limit}` | `TTL_PRODUCT_LIST` |
| GET | `/products/top?limit=10` | Top produk terlaris (query agregasi berat) | `stats:top_products:l:{limit}` | `TTL_TOP_PRODUCTS` |
| GET | `/stats/dashboard` | Ringkasan: total order, total revenue, order hari ini, produk terjual | `stats:dashboard` | `TTL_DASHBOARD` |

> Catatan routing: daftarkan `/products/top` **sebelum** `/products/{id}` agar tidak bentrok.

**Query SQL untuk `top products`** (sengaja berat):

```sql
SELECT p.id, p.name, p.price, SUM(oi.quantity) AS total_sold
FROM order_items oi
JOIN products p ON p.id = oi.product_id
GROUP BY p.id, p.name, p.price
ORDER BY total_sold DESC
LIMIT ?;
```

**Query SQL untuk `dashboard`**:

```sql
-- total order & total revenue
SELECT COUNT(*) AS total_orders, COALESCE(SUM(total_amount), 0) AS total_revenue
FROM orders WHERE status <> 'cancelled';

-- order hari ini
SELECT COUNT(*) FROM orders WHERE DATE(created_at) = CURDATE();

-- total item terjual
SELECT COALESCE(SUM(quantity), 0) FROM order_items;
```

### 8.2 Endpoint tulis (tidak di-cache, tapi melakukan invalidasi)

| Method | Path | Deskripsi | Invalidasi |
|---|---|---|---|
| POST | `/products` | Buat produk baru | naikkan versi list (`INCR products:list:version`) |
| PUT | `/products/{id}` | Update produk | `DEL product:{id}` + `INCR products:list:version` |
| DELETE | `/products/{id}` | Hapus produk | `DEL product:{id}` + `INCR products:list:version` |
| POST | `/orders` | Buat pesanan baru (beserta items) | **Tidak** invalidasi top/dashboard (lihat catatan) |

**Catatan sengaja untuk `POST /orders`:** cache `top_products` dan `dashboard` **tidak** di-invalidasi, hanya mengandalkan TTL. Ini mendemonstrasikan konsep *eventual consistency*: data statistik boleh telat beberapa menit. Dokumentasikan keputusan ini di README.

### 8.3 Endpoint pendukung

| Method | Path | Deskripsi |
|---|---|---|
| GET | `/health` | Cek koneksi MySQL & Redis, return status masing-masing |
| DELETE | `/debug/cache` | Flush seluruh cache (`FLUSHDB`), hanya untuk keperluan benchmark. Aktif hanya jika `APP_ENV=development` |

### 8.4 Format response

Sukses:

```json
{ "data": { ... } }
```

Error:

```json
{ "error": { "message": "product not found" } }
```

Status code: `200`, `201`, `400`, `404`, `429` (rate limit), `500`.

### 8.5 Header debug (wajib)

Setiap endpoint baca yang memakai cache **harus** mengirim header:

```
X-Cache: HIT     # data dari Redis
X-Cache: MISS    # data dari MySQL
X-Cache: BYPASS  # CACHE_ENABLED=false
```

Tambahkan juga header `X-Response-Time: 3.2ms` (durasi pemrosesan di server).

---

## 9. Strategi Cache

### 9.1 Pola: cache-aside (di service layer)

```
func GetProduct(ctx, id):
    if CACHE_ENABLED:
        data, found = cache.GetJSON("product:{id}")
        if found: return data, HIT
    data = repo.FindByID(id)            // MySQL
    if CACHE_ENABLED:
        cache.SetJSON("product:{id}", data, TTL)
    return data, MISS
```

### 9.2 Aturan penamaan key

- Format: `namespace:entity:id[:qualifier]`, dipisah titik dua.
- Semua huruf kecil.
- Contoh: `product:42`, `stats:dashboard`, `products:list:v3:cat:5:p:1:l:20`.

### 9.3 Serialisasi

- Simpan value sebagai **JSON string** (`encoding/json`).
- Buat helper generic di `internal/cache/cache.go`:
  - `GetJSON[T any](ctx, key) (T, bool, error)`
  - `SetJSON(ctx, key, value, ttl) error`
  - `Delete(ctx, keys ...string) error`
  - `IncrVersion(ctx, key) (int64, error)`

### 9.4 Invalidasi list produk dengan "version key"

Menghapus banyak key list (berbagai kategori/halaman) itu sulit. Gunakan pola **versioning**:

1. Simpan counter di Redis: `products:list:version` (default 1 jika belum ada).
2. Versi tersebut menjadi bagian dari key list: `products:list:v{ver}:...`.
3. Setiap ada create/update/delete produk → `INCR products:list:version`.
4. Key lama otomatis tidak terpakai lagi dan hilang sendiri oleh TTL.

Jangan gunakan `KEYS *` untuk invalidasi (berbahaya dan lambat).

### 9.5 Toleransi kegagalan Redis (penting)

- Jika Redis down atau error, **aplikasi tidak boleh crash**.
- Error dari Redis hanya di-log (`slog.Warn`), lalu fallback langsung ke MySQL.
- Gunakan timeout pendek untuk operasi Redis (misal 100–200 ms) lewat `context.WithTimeout`.
- Redis **hanya** optimasi, bukan dependensi kritis (kecuali rate limiter, lihat tahap 5).

### 9.6 Cache stampede (opsional, tahap 6)

Saat key populer expired, banyak request bersamaan bisa menembak MySQL sekaligus. Mitigasi:

- Gunakan `golang.org/x/sync/singleflight` agar hanya satu goroutine yang query DB untuk key yang sama.
- Tambahkan **jitter** acak ±10% pada TTL.

---

## 10. Rate Limiter (Bonus, Tahap 5)

Middleware `internal/middleware/ratelimit.go` dengan algoritma **fixed window counter**:

- Key: `ratelimit:{client_ip}:{unix_minute}`
- Logika: `INCR` key; jika hasilnya 1 maka `EXPIRE key 60`.
- Batas: **100 request per menit per IP** (konfigurasi lewat env `RATE_LIMIT_PER_MIN`).
- Jika melebihi batas → status `429` + header `Retry-After`.
- Tambahkan header `X-RateLimit-Limit` dan `X-RateLimit-Remaining`.
- Jika Redis error → **fail open** (request tetap diizinkan) dan log warning.

---

## 11. Rencana Benchmark

Gunakan [`hey`](https://github.com/rakyll/hey). Jalankan tiap skenario dan catat hasilnya di `README.md` dalam bentuk tabel.

### Skenario

| # | Endpoint | CACHE_ENABLED | Perintah |
|---|---|---|---|
| 1 | `/products/top` | false | `hey -n 1000 -c 50 http://192.168.54.202:8080/products/top` |
| 2 | `/products/top` | true (setelah warm-up) | perintah sama |
| 3 | `/stats/dashboard` | false | `hey -n 1000 -c 50 http://192.168.54.202:8080/stats/dashboard` |
| 4 | `/stats/dashboard` | true | perintah sama |
| 5 | `/products/1` | false | `hey -n 5000 -c 100 http://192.168.54.202:8080/products/1` |
| 6 | `/products/1` | true | perintah sama |

### Metrik yang dicatat

- Requests/sec
- Latency rata-rata, p50, p95, p99
- Jumlah error

### Template tabel hasil (isi di README)

| Endpoint | Cache | Req/s | Avg | p95 | p99 |
|---|---|---|---|---|---|
| /products/top | OFF | | | | |
| /products/top | ON | | | | |

---

## 12. Tahapan Implementasi

Kerjakan **berurutan**. Setelah tiap tahap, pastikan `go build ./...` sukses dan fitur bisa dites manual.

### Tahap 1: Fondasi tanpa Redis
- Setup `go.mod`, struktur folder, config, koneksi MySQL, `.env.example`.
- Migration SQL dan seeder.
- Endpoint: `GET /products/{id}`, `GET /products`, `GET /products/top`, `GET /stats/dashboard`, `GET /health`.
- Middleware logger (durasi request).
- **Selesai jika:** semua endpoint berjalan murni dari MySQL; hasil benchmark baseline dicatat.

### Tahap 2: Cache-aside untuk detail produk
- Setup `docker-compose.yml`, koneksi Redis, helper `cache.go`.
- Terapkan cache-aside di `GET /products/{id}`.
- Header `X-Cache` dan `X-Response-Time`.
- Toggle `CACHE_ENABLED`.
- **Selesai jika:** request pertama `MISS`, request kedua `HIT`, dan key terlihat di `redis-cli`.

### Tahap 3: Cache untuk endpoint agregasi & list
- Cache `/products/top`, `/stats/dashboard`, `/products` (list dengan version key).
- **Selesai jika:** latency endpoint agregasi turun drastis pada request kedua dan seterusnya.

### Tahap 4: Endpoint tulis & invalidasi
- `POST /products`, `PUT /products/{id}`, `DELETE /products/{id}`, `POST /orders`.
- Invalidasi sesuai bagian 8.2.
- Endpoint `DELETE /debug/cache`.
- **Selesai jika:** setelah `PUT /products/1`, `GET /products/1` mengembalikan data terbaru (MISS lalu HIT), dan list produk ikut ter-refresh.

### Tahap 5: Rate limiter
- Middleware rate limit sesuai bagian 10.
- **Selesai jika:** request ke-101 dalam semenit dari IP yang sama mendapat `429`.

### Tahap 6 (opsional): Hardening
- Singleflight dan TTL jitter (bagian 9.6).
- Timeout operasi Redis dan fallback saat Redis down (verifikasi dengan `docker stop redis-case-study`, API harus tetap jalan).
- Graceful shutdown server.

### Tahap 7: Dokumentasi
- `README.md`: cara setup, cara menjalankan, penjelasan keputusan cache (data apa, kenapa, TTL), dan tabel hasil benchmark.

---

## 13. Kriteria Penerimaan (Acceptance Criteria)

- [ ] `make redis-up && make migrate && make seed && make run` berjalan tanpa error.
- [ ] Semua endpoint di bagian 8 berfungsi sesuai spesifikasi.
- [ ] Header `X-Cache` menunjukkan `MISS` lalu `HIT` secara konsisten.
- [ ] `CACHE_ENABLED=false` membuat semua request langsung ke MySQL (`X-Cache: BYPASS`).
- [ ] Update/hapus produk membuat cache terkait tidak lagi mengembalikan data lama.
- [ ] Mematikan container Redis **tidak** membuat API crash (fallback ke MySQL).
- [ ] Tidak ada penggunaan `KEYS *` di kode aplikasi.
- [ ] Endpoint agregasi menunjukkan peningkatan performa yang terukur saat cache aktif.
- [ ] Error ditangani dengan jelas (tidak ada panic, tidak ada error yang diabaikan diam-diam).

---

## 14. Aturan Kode untuk AI Generator

1. Tulis **kode lengkap dan siap jalan**, bukan pseudocode atau skeleton.
2. Gunakan `context.Context` di semua operasi DB dan Redis.
3. Gunakan **prepared statement / parameterized query**; jangan menyusun SQL dengan string concatenation dari input user.
4. Tangani semua error dan bungkus dengan konteks (`fmt.Errorf("...: %w", err)`).
5. Jangan menaruh logika bisnis di handler; handler hanya parsing request dan menulis response.
6. Komentar singkat hanya untuk bagian yang tidak jelas dengan sendirinya.
7. Ikuti struktur folder di bagian 4 persis.
8. Setelah selesai tiap tahap, tampilkan ringkasan: file yang dibuat/diubah dan cara mengetesnya.
