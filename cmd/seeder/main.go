package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/database"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "Only run database migration scripts without seeding data")
	totalCategories := flag.Int("categories", 20, "Number of categories to seed")
	totalProducts := flag.Int("products", 5000, "Number of products to seed")
	totalOrders := flag.Int("orders", 200000, "Number of orders to seed")
	batchSize := flag.Int("batch-size", 1000, "Batch size for bulk inserts")
	flag.Parse()

	cfg := config.Load()
	slog.Info("Connecting to MySQL...", "dsn", cfg.MySQLDSN)

	db, err := database.NewMySQL(cfg)
	if err != nil {
		slog.Error("Failed to connect to MySQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("Running migration migrations/001_init.sql...")
	if err := runMigration(db, "migrations/001_init.sql"); err != nil {
		slog.Error("Migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Migration finished successfully")

	if *migrateOnly {
		slog.Info("Migrate-only mode requested. Exiting.")
		return
	}

	start := time.Now()
	slog.Info("Starting database seeder...",
		"categories", *totalCategories,
		"products", *totalProducts,
		"orders", *totalOrders,
		"batch_size", *batchSize,
	)

	ctx := context.Background()

	// Clean existing data
	slog.Info("Cleaning existing tables...")
	cleanSQL := `
		SET FOREIGN_KEY_CHECKS = 0;
		TRUNCATE TABLE order_items;
		TRUNCATE TABLE orders;
		TRUNCATE TABLE products;
		TRUNCATE TABLE categories;
		SET FOREIGN_KEY_CHECKS = 1;
	`
	for _, query := range strings.Split(cleanSQL, ";") {
		q := strings.TrimSpace(query)
		if q != "" {
			if _, err := db.ExecContext(ctx, q); err != nil {
				slog.Error("Failed to truncate table", "query", q, "error", err)
				os.Exit(1)
			}
		}
	}

	// 1. Seed Categories
	slog.Info("Seeding categories...")
	categoryNames := []string{
		"Elektronik", "Komputer & Laptop", "Handphone & Tablet", "Kamera",
		"Pakaian Pria", "Pakaian Wanita", "Sepatu", "Tas & Aksesoris",
		"Kesehatan", "Kecantikan", "Ibu & Bayi", "Makanan & Minuman",
		"Otomotif", "Olahraga", "Buku & Alat Tulis", "Hobi & Koleksi",
		"Perlengkapan Rumah", "Dapur", "Pertukangan", "Tiket & Voucher",
	}

	for i := 0; i < *totalCategories; i++ {
		name := fmt.Sprintf("Kategori %d", i+1)
		if i < len(categoryNames) {
			name = categoryNames[i]
		}
		_, err := db.ExecContext(ctx, "INSERT INTO categories (name) VALUES (?)", name)
		if err != nil {
			slog.Error("Failed to insert category", "error", err)
			os.Exit(1)
		}
	}
	slog.Info("Categories seeded", "count", *totalCategories)

	// 2. Seed Products
	slog.Info("Seeding products...")
	if err := seedProducts(ctx, db, *totalProducts, *totalCategories, *batchSize); err != nil {
		slog.Error("Failed to seed products", "error", err)
		os.Exit(1)
	}

	// 3. Seed Orders & Order Items
	slog.Info("Seeding orders and order items...")
	if err := seedOrders(ctx, db, *totalOrders, *totalProducts, *batchSize); err != nil {
		slog.Error("Failed to seed orders", "error", err)
		os.Exit(1)
	}

	slog.Info("All data seeded successfully!", "elapsed_time", time.Since(start).String())
}

func runMigration(db *sql.DB, migrationFile string) error {
	content, err := os.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	queries := strings.Split(string(content), ";")
	for _, query := range queries {
		q := strings.TrimSpace(query)
		if q == "" {
			continue
		}
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed executing query [%s]: %w", q, err)
		}
	}
	return nil
}

func seedProducts(ctx context.Context, db *sql.DB, total, totalCategories, batchSize int) error {
	baseTime := time.Now().AddDate(-1, 0, 0)
	inserted := 0

	for inserted < total {
		currentBatch := batchSize
		if inserted+currentBatch > total {
			currentBatch = total - inserted
		}

		valueStrings := make([]string, 0, currentBatch)
		valueArgs := make([]any, 0, currentBatch*6)

		for i := 0; i < currentBatch; i++ {
			pNum := inserted + i + 1
			catID := rand.Intn(totalCategories) + 1
			name := fmt.Sprintf("Produk Unggulan %d", pNum)
			desc := fmt.Sprintf("Deskripsi lengkap dan spesifikasi untuk %s", name)
			// Price between 10.000 and 5.000.000 rounded to 1.000
			price := float64((rand.Intn(4991)+10)*1000)
			stock := rand.Intn(491) + 10
			createdAt := baseTime.Add(time.Duration(rand.Int63n(int64(365 * 24 * time.Hour))))

			valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?)")
			valueArgs = append(valueArgs, catID, name, desc, price, stock, createdAt)
		}

		stmt := fmt.Sprintf("INSERT INTO products (category_id, name, description, price, stock, created_at) VALUES %s",
			strings.Join(valueStrings, ","))

		if _, err := db.ExecContext(ctx, stmt, valueArgs...); err != nil {
			return fmt.Errorf("failed batch insert products: %w", err)
		}

		inserted += currentBatch
		if inserted%1000 == 0 || inserted == total {
			fmt.Printf("Products progress: %d/%d (%.1f%%)\r", inserted, total, float64(inserted)/float64(total)*100)
		}
	}
	fmt.Println()
	return nil
}

func seedOrders(ctx context.Context, db *sql.DB, total, totalProducts, batchSize int) error {
	names := []string{
		"Budi Santoso", "Siti Aminah", "Ahmad Fauzi", "Dewi Lestari", "Eko Prasetyo",
		"Rina Wijaya", "Agus Setiawan", "Nurul Hidayah", "Hendra Gunawan", "Fitri Handayani",
		"Doni Pratama", "Maya Anggraini", "Rizky Ramadhan", "Dian Sastro", "Fajar Hidayat",
	}

	baseTime := time.Now().AddDate(-1, 0, 0)
	insertedOrders := 0
	totalOrderItems := 0

	for insertedOrders < total {
		currentBatch := batchSize
		if insertedOrders+currentBatch > total {
			currentBatch = total - insertedOrders
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin tx: %w", err)
		}

		orderValueStrings := make([]string, 0, currentBatch)
		orderValueArgs := make([]any, 0, currentBatch*4)

		type tempOrderItem struct {
			orderOffset int
			productID   uint64
			quantity    int
			unitPrice   float64
		}
		batchItems := make([]tempOrderItem, 0, currentBatch*3)

		for i := 0; i < currentBatch; i++ {
			customer := names[rand.Intn(len(names))]
			createdAt := baseTime.Add(time.Duration(rand.Int63n(int64(365 * 24 * time.Hour))))

			// 1 to 5 random items per order
			itemCount := rand.Intn(5) + 1
			var totalAmount float64

			for j := 0; j < itemCount; j++ {
				prodID := uint64(rand.Intn(totalProducts) + 1)
				qty := rand.Intn(3) + 1
				unitPrice := float64((rand.Intn(500) + 10) * 1000)
				totalAmount += unitPrice * float64(qty)

				batchItems = append(batchItems, tempOrderItem{
					orderOffset: i,
					productID:   prodID,
					quantity:    qty,
					unitPrice:   unitPrice,
				})
			}

			orderValueStrings = append(orderValueStrings, "(?, ?, 'paid', ?)")
			orderValueArgs = append(orderValueArgs, customer, totalAmount, createdAt)
		}

		orderStmt := fmt.Sprintf("INSERT INTO orders (customer_name, total_amount, status, created_at) VALUES %s",
			strings.Join(orderValueStrings, ","))

		res, err := tx.ExecContext(ctx, orderStmt, orderValueArgs...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed batch insert orders: %w", err)
		}

		firstOrderID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to get first order id: %w", err)
		}

		// Insert order items for this batch
		if len(batchItems) > 0 {
			itemValueStrings := make([]string, 0, len(batchItems))
			itemValueArgs := make([]any, 0, len(batchItems)*4)

			for _, bi := range batchItems {
				actualOrderID := firstOrderID + int64(bi.orderOffset)
				itemValueStrings = append(itemValueStrings, "(?, ?, ?, ?)")
				itemValueArgs = append(itemValueArgs, actualOrderID, bi.productID, bi.quantity, bi.unitPrice)
			}

			itemStmt := fmt.Sprintf("INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES %s",
				strings.Join(itemValueStrings, ","))

			if _, err := tx.ExecContext(ctx, itemStmt, itemValueArgs...); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed batch insert order items: %w", err)
			}
			totalOrderItems += len(batchItems)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit order batch: %w", err)
		}

		insertedOrders += currentBatch
		if insertedOrders%5000 == 0 || insertedOrders == total {
			fmt.Printf("Orders: %d/%d (%.1f%%) | Items: %d\r",
				insertedOrders, total, float64(insertedOrders)/float64(total)*100, totalOrderItems)
		}
	}
	fmt.Println()
	return nil
}
