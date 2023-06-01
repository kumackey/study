package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"golang.org/x/text/encoding/japanese"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var pageURLFormat = "https://www.aozora.gr.jp/cards/%s/card%s.html"

type Entry struct {
	AuthorID string
	Author   string
	TitleID  string
	Title    string
	SiteURL  string
	ZipURL   string
}

func main() {
	db, err := setupDB("datebase.sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"
	entries, err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d entries\n", len(entries))

	for _, entry := range entries {
		log.Printf("adding %v\n", entry)
		content, err := extractText(entry.ZipURL)
		if err != nil {
			log.Println(err)
			continue
		}

		err = addEntry(db, &entry, content)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}

func findEntries(siteURL string) ([]Entry, error) {
	doc, err := goquery.NewDocument(siteURL)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0)
	pat := regexp.MustCompile(`.*/cards/([0-9]+)/card([0-9]+)\.html$`)
	doc.Find("ol li a").Each(func(i int, elem *goquery.Selection) {
		token := pat.FindStringSubmatch(elem.AttrOr("href", ""))
		if len(token) != 3 {
			return
		}

		author, zipURL := findAuthorAndZip(
			fmt.Sprintf(pageURLFormat, token[1], token[2]),
		)

		if zipURL != "" {
			entries = append(entries, Entry{
				AuthorID: token[1],
				Author:   author,
				TitleID:  token[2],
				Title:    elem.Text(),
				SiteURL:  siteURL,
				ZipURL:   zipURL,
			})
		}
	})

	return entries, nil
}

func findAuthorAndZip(siteURL string) (string, string) {
	log.Println("query", siteURL)
	doc, err := goquery.NewDocument(siteURL)
	if err != nil {
		return "", ""
	}

	zipURL := ""

	author := doc.Find("table[summary=作家データ] tr:nth-child(2) td:nth-child(2)").Text()
	doc.Find("table.download a").Each(func(n int, elem *goquery.Selection) {
		href := elem.AttrOr("href", "")
		if strings.HasSuffix(href, ".zip") {
			zipURL = href
		}
	})

	if zipURL == "" {
		return author, ""
	}
	if strings.HasPrefix(zipURL, "https://") || strings.HasPrefix(zipURL, "http://") {
		// 初めから絶対パスで指定されているケース
		return author, zipURL
	}

	u, err := url.Parse(siteURL)
	if err != nil {
		return author, ""
	}

	u.Path = path.Join(path.Dir(u.Path), zipURL)

	return author, u.String()
}
func extractText(zipURL string) (string, error) {
	resp, err := http.Get(zipURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil
	}

	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}

	for _, file := range r.File {
		if path.Ext(file.Name) == ".txt" {
			f, err := file.Open()
			if err != nil {
				return "", err
			}
			b, err := ioutil.ReadAll(f)
			f.Close()
			if err != nil {
				return "", err
			}
			b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
			if err != nil {
				return "", err
			}

			return string(b), nil
		}
	}

	return "", errors.New("contents not found")
}

func setupDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS authors(author_id TEXT, author TEXT, PRIMARY KEY(author_id))`,
		`CREATE TABLE IF NOT EXISTS contents(author_id TEXT, title_id TEXT, title TEXT, content TEXT, PRIMARY KEY(author_id, title_id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS contents_fts USING fts4(words)`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}

func addEntry(db *sql.DB, entry *Entry, content string) error {
	_, err := db.Exec(`REPLACE INTO authors(author_id, author) VALUES(?, ?)`,
		entry.AuthorID,
		entry.Author,
	)
	if err != nil {
		return err
	}

	b, err := os.ReadFile("ababababa.txt")
	if err != nil {
		return err
	}

	b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
	if err != nil {
		return err
	}

	res, err := db.Exec(`REPLACE INTO contents(author_id, title_id, title, content) VALUES(?, ?, ?, ?)`,
		entry.AuthorID,
		entry.TitleID,
		entry.Title,
		content,
	)
	if err != nil {
		return err
	}

	docID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return err
	}

	seg := t.Wakati(content)
	_, err = db.Exec(`REPLACE INTO contents_fts(docid, words) VALUES(?, ?)`, docID, strings.Join(seg, " "))
	if err != nil {
		return err
	}

	return nil
}