package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "testing"
    _ "github.com/lib/pq" // PostgreSQL driver
)

const (
    connStr = "user=youruser dbname=yourdb sslmode=disable" // replace with your actual connection string
)

func TestPasswordValidationWithPostgres(t *testing.T) {
    // Connect to the database
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        t.Fatalf("Failed to connect to database: %v", err)
    }
    defer db.Close()

    ctx := context.Background()

    // Create a table for testing
    _, err = db.ExecContext(ctx, `
        CREATE TEMPORARY TABLE users (
            id SERIAL PRIMARY KEY,
            username VARCHAR(255),
            password VARCHAR(255)
        )
    `)
    if err != nil {
        t.Fatalf("Failed to create temporary table: %v", err)
    }

    // Define test cases
    tests := []struct {
        password   string
        expectErr  bool
    }{
        {"short", true},                // Password too short
        {"ValidP@ssw0rd", false},       // Valid password
        {"WithoutUpperCase1!", true},   // Missing uppercase letter
        {"withoutlowercase1!", true},   // Missing lowercase letter
        {"WithoutDigit!", true},        // Missing digit
        {"WithoutSpecial1", true},      // Missing special character
        {string(make([]byte, maxPasswordLength+1)), true}, // Password too long
    }

    for _, tt := range tests {
        t.Run(fmt.Sprintf("password: %s", tt.password), func(t *testing.T) {
            err := ValidatePassword(tt.password)
            if (err != nil) != tt.expectErr {
                t.Errorf("ValidatePassword(%s) = %v, expected error = %v", tt.password, err, tt.expectErr)
            }

            if !tt.expectErr {
                // Insert the valid password into the database
                _, err = db.ExecContext(ctx, "INSERT INTO users (username, password) VALUES ($1, $2)", "testuser", tt.password)
                if err != nil {
                    t.Fatalf("Failed to insert password into database: %v", err)
                }

                // Retrieve the password back from the database
                var storedPassword string
                err = db.QueryRowContext(ctx, "SELECT password FROM users WHERE username=$1", "testuser").Scan(&storedPassword)
                if err != nil {
                    t.Fatalf("Failed to retrieve password from database: %v", err)
                }

                // Check if the stored password matches the original
                if storedPassword != tt.password {
                    t.Errorf("Stored password does not match original. Got %s, want %s", storedPassword, tt.password)
                }
            }
        })
    }
}
