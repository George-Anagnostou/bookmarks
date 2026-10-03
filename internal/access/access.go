// Package access defines the concepts of Person and Client, and handles how
// users and their clients connect to a shared bookmarkd server and database.
package access

import "time"

type Person struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type Client struct {
	ID             string
	PersonID       string
	Name           string
	TokenHash      string
	CreatedAt      time.Time
	TokenRevokedAt *time.Time
}

/*
CREATE TABLE IF NOT EXISTS person (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS client (
	id TEXT PRIMARY KEY,
	person_id TEXT NOT NULL REFERENCES person(id),
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	token_revoked_at TEXT
) STRICT;
*/

// func (s *Store) CreatePerson(name string) (Person, error)
//
// func FindPerson(id string) (*Person, error)
