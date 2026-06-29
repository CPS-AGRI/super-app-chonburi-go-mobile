package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Try loading env from multiple possible relative paths
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN environment variable is empty or not loaded")
	}

	fmt.Printf("Connecting to database using DSN: %s\n", dsn[:30]+"... (redacted)")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("Successfully connected to database.")

	// Check if table users exists
	hasUsers := db.Migrator().HasTable("users")
	fmt.Printf("Table 'users' exists: %v\n", hasUsers)

	if hasUsers {
		// List all columns in the users table
		columns, err := db.Migrator().ColumnTypes("users")
		if err != nil {
			log.Fatalf("Failed to get columns: %v", err)
		}

		fmt.Println("\nColumns in table 'users':")
		for _, col := range columns {
			nullable, _ := col.Nullable()
			dbType := col.DatabaseTypeName()
			size, _ := col.Length()
			fmt.Printf("- %s (%s, nullable: %v, size: %v)\n", col.Name(), dbType, nullable, size)
		}
	}
}
