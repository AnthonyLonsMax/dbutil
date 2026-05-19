// Example: Blog API — 3 levels (Author → Post → Comment)
//
// Demonstrates the full pipeline using sqlx:
//   1. Query authors
//   2. Extract IDs, query posts WHERE author_id IN (...)
//   3. Extract IDs, query comments WHERE post_id IN (...)
//   4. Reconstruct bottom-up: comments → posts → authors
//
// Run: go run ./_examples/blog/

package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/AnthonyLonsMax/dbutil"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type Comment struct {
	ID     int    `json:"id" db:"id"`
	PostID int    `json:"post_id" db:"post_id"`
	Author string `json:"author" db:"author"`
	Body   string `json:"body" db:"body"`
}

type Post struct {
	ID       int       `json:"id" db:"id"`
	AuthorID int       `json:"author_id" db:"author_id"`
	Title    string    `json:"title" db:"title"`
	Comments []Comment `json:"comments"`
}

type Author struct {
	ID    int    `json:"id" db:"id"`
	Name  string `json:"name" db:"name"`
	Posts []Post `json:"posts"`
}

func main() {
	db := sqlx.MustOpen("sqlite3", ":memory:")
	defer db.Close()

	db.MustExec(`
		CREATE TABLE authors (id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE posts (id INTEGER PRIMARY KEY, author_id INTEGER REFERENCES authors(id), title TEXT);
		CREATE TABLE comments (id INTEGER PRIMARY KEY, post_id INTEGER REFERENCES posts(id), author TEXT, body TEXT);
	`)

	db.MustExec(`
		INSERT INTO authors VALUES (1, 'Alice'), (2, 'Bob');
		INSERT INTO posts VALUES (1, 1, 'First Post'), (2, 1, 'Second Post'), (3, 2, 'Hello World');
		INSERT INTO comments VALUES
			(1, 1, 'Charlie', 'Great post!'),
			(2, 1, 'Diana', 'Thanks!'),
			(3, 2, 'Eve', 'Nice write-up'),
			(4, 3, 'Frank', 'First comment!'),
			(5, 3, 'Grace', 'Awesome blog');
	`)

	// ---- Phase 1: Query top-down ----

	var authors []Author
	if err := db.Select(&authors, "SELECT id, name FROM authors ORDER BY id"); err != nil {
		log.Fatal(err)
	}

	authorIDs := dbutil.ExtractIDs(authors, func(a Author) int { return a.ID })
	q, args, err := sqlx.In("SELECT id, author_id, title FROM posts WHERE author_id IN (?) ORDER BY id", authorIDs)
	if err != nil {
		log.Fatal(err)
	}
	var posts []Post
	if err := db.Select(&posts, q, args...); err != nil {
		log.Fatal(err)
	}

	postIDs := dbutil.ExtractIDs(posts, func(p Post) int { return p.ID })
	q, args, err = sqlx.In("SELECT id, post_id, author, body FROM comments WHERE post_id IN (?) ORDER BY id", postIDs)
	if err != nil {
		log.Fatal(err)
	}
	var comments []Comment
	if err := db.Select(&comments, q, args...); err != nil {
		log.Fatal(err)
	}

	// ---- Phase 2: Reconstruct bottom-up ----

	dbutil.MergeChildren(
		posts, dbutil.GroupBy(comments, func(c Comment) int { return c.PostID }),
		func(p Post) int { return p.ID },
		func(p *Post, cs []Comment) { p.Comments = cs },
	)

	dbutil.MergeChildren(
		authors, dbutil.GroupBy(posts, func(p Post) int { return p.AuthorID }),
		func(a Author) int { return a.ID },
		func(a *Author, ps []Post) { a.Posts = ps },
	)

	out, _ := json.MarshalIndent(authors, "", "  ")
	fmt.Println(string(out))
}
