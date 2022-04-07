// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"testing"
)

type DBType int

var DBTypeSQLite DBType = 1
var DBTypePostgres DBType = 2

var Quiet = false

func createLocalDB(dbName string) string {
	if !Quiet {
		fmt.Println("Note: tests require a postgres install accessible to the current user")
	}
	createDB := exec.Command("createdb", dbName)
	if !Quiet {
		createDB.Stdout = os.Stdout
		createDB.Stderr = os.Stderr
	}
	err := createDB.Run()
	if err != nil && !Quiet {
		fmt.Println("createLocalDB returned error:", err)
	}
	return dbName
}

func currentUser() string {
	user, err := user.Current()
	if err != nil {
		if !Quiet {
			fmt.Println("cannot get current user: ", err)
		}
		os.Exit(2)
	}
	return user.Username
}

// Prepare a sqlite or postgres connection string for testing.
// Returns the connection string to use and a close function which must be called when the test finishes.
// Calling this function twice will return the same database, which will have data from previous tests
// unless close() is called.
// TODO: namespace for concurrent package tests
func PrepareDBConnectionString(t *testing.T, dbType DBType) (connStr string, close func()) {
	if dbType == DBTypeSQLite {
		dbname := "dendrite_test.db"
		return fmt.Sprintf("file:%s", dbname), func() {
			err := os.Remove(dbname)
			if err != nil {
				t.Fatalf("failed to cleanup sqlite db '%s': %s", dbname, err)
			}
		}
	}

	// Required vars: user and db
	// We'll try to infer from the local env if they are missing
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = currentUser()
	}
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = createLocalDB("dendrite_test")
	}
	connStr = fmt.Sprintf(
		"user=%s dbname=%s sslmode=disable",
		user, dbName,
	)
	// optional vars, used in CI
	password := os.Getenv("POSTGRES_PASSWORD")
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	host := os.Getenv("POSTGRES_HOST")
	if host != "" {
		connStr += fmt.Sprintf(" host=%s", host)
	}

	return connStr, func() {
		// Drop all tables on the database to get a fresh instance
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			t.Fatalf("failed to connect to postgres db '%s': %s", connStr, err)
		}
		_, err = db.Exec(`DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;`)
		if err != nil {
			t.Fatalf("failed to cleanup postgres db '%s': %s", connStr, err)
		}
		_ = db.Close()
	}
}

// Creates subtests with each known DBType
func WithAllDatabases(t *testing.T, testFn func(t *testing.T, db DBType)) {
	dbs := map[string]DBType{
		"postgres": DBTypePostgres,
		"sqlite":   DBTypeSQLite,
	}
	for dbName, dbType := range dbs {
		dbt := dbType
		t.Run(dbName, func(tt *testing.T) {
			testFn(tt, dbt)
		})
	}
}
