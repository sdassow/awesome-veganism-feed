package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/feeds"
	"github.com/natefinch/atomic"
)

func main() {
	var destdir string
	var workdir string
	var stylesheet string
	var verbose bool

	flag.StringVar(&destdir, "destdir", ".", "destination directory for feed files")
	flag.StringVar(&workdir, "workdir", ".", "working directory with a git repository")
	flag.StringVar(&stylesheet, "stylesheet", "", "xslt stylesheet to inject into atom feed")
	flag.BoolVar(&verbose, "verbose", false, "turn on verbose mode")
	flag.Parse()

	// regular expression to find relevant items in diffs
	re, err := regexp.Compile(`\n([+-])\s*[-] \[([^\]]+)\]\(([^\)]+)\) [-] ([^\n]+)`)
	if err != nil {
		log.Fatalf("failed to compile regular expression: %v", err)
	}

	// open checked out repository
	r, err := git.PlainOpen(workdir)
	if err != nil {
		log.Fatalf("failed to open repository: %s: %v", workdir, err)
	}

	// file to work with
	workfile := "README.md"

	// make sure file exists
	if _, err := os.Stat(filepath.Join(workdir, workfile)); err != nil {
		log.Fatalf("failed to locate file: %v", err)
	}

	// get HEAD reference
	ref, err := r.Head()
	if err != nil {
		log.Fatalf("failed to get HEAD reference: %v", err)
	}

	logopts := &git.LogOptions{
		From:     ref.Hash(),
		FileName: &workfile,
		Order:    git.LogOrderCommitterTime,
	}

	// get commit history
	iter, err := r.Log(logopts)
	if err != nil {
		log.Fatalf("failed to get log: %v", err)
	}

	// build list with all commits
	var commits []*object.Commit
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)

		return nil
	})
	if err != nil {
		log.Fatalf("failed to iterate commit log: %v", err)
	}

	if len(commits) == 0 {
		log.Fatal("failed to find commits")
	}

	// setup feed
	feed := &feeds.Feed{
		Title:       "Awesome Veganism Feed",
		Link:        &feeds.Link{Href: "https://awesome-veganism.com/"},
		Description: "A curated list of awesome resources, pointers, and tips to make veganism easy and accessible to everyone.",
		Created: commits[len(commits)-1].Author.When,
	}

	for n := len(commits) - 1; n >= 0; n-- {
		c := commits[n]

		// skip initial commit in this project as it happens to have no relevant content
		if n == 0 {
			break
		}

		p := commits[n-1]

		if verbose {
			log.Printf("===> commit: %s by %s at %s: %s", p.Hash, p.Author.Name, p.Author.When, p.Message)
		}

		patch, err := c.Patch(p)
		if err != nil {
			log.Fatalf("failed to get patch: %v", err)
		}

		matches := re.FindAllStringSubmatch(patch.String(), -1)

		// filter out moving items around: a plus and a minus cancel each other out
		changes := make(map[string]int)
		for _, m := range matches {
			x := 1
			if m[1] == "-" {
				x = -1
			}

			v, found := changes[m[2]]
			if !found {
				v = x
			} else {
				v += x
			}

			changes[m[2]] = v
		}

		if verbose {
			log.Printf("changes: %v", changes)
		}

		for _, m := range matches {
			// skip when there was only a move of an entry
			// safe to access without check due to full iteration in previous loop
			if changes[m[2]] == 0 {
				continue
			}

			t := "Addition"
			if m[1] == "-" {
				t = "Removal"
			}

			if verbose {
				log.Printf("=====>> %s: %s -- %s -- %s", t, m[2], m[3], m[4])
			}

			feed.Items = append(feed.Items, &feeds.Item{
				Title:       fmt.Sprintf("%s of %s", t, m[2]),
				Link:        &feeds.Link{Href: m[3]},
				Description: m[4],
				Author:      &feeds.Author{Name: p.Author.Name},
				Created:     p.Author.When,
			})

			feed.Updated = p.Author.When
		}
	}

	atom, err := feed.ToAtom()
	if err != nil {
		log.Fatalf("failed to generate atom feed: %v", err)
	}
	if stylesheet != "" {
		atom = injectAtomStylesheet(atom, stylesheet)
	}
	atom = adjustAtomLinks(atom, "feed.xml")
	if err := atomic.WriteFile(filepath.Join(destdir, "feed.xml"), bytes.NewReader([]byte(atom))); err != nil {
		log.Fatalf("failed to write atom feed: %v", err)
	}

	json, err := feed.ToJSON()
	if err != nil {
		log.Fatalf("failed to generate json feed: %v", err)
	}
	if err := atomic.WriteFile(filepath.Join(destdir, "feed.json"), bytes.NewReader([]byte(json))); err != nil {
		log.Fatalf("failed to write json feed: %v", err)
	}

	rss, err := feed.ToRss()
	if err != nil {
		log.Fatalf("failed to generate rss feed: %v", err)
	}
	rss = adjustRssAuthors(rss)
	if err := atomic.WriteFile(filepath.Join(destdir, "feed.rss"), bytes.NewReader([]byte(rss))); err != nil {
		log.Fatalf("failed to write rss feed: %v", err)
	}

	files := []string{
		filepath.Join(destdir, "feed.xml"),
		filepath.Join(destdir, "feed.json"),
		filepath.Join(destdir, "feed.rss"),
	}
	for _, f := range files {
		if err := os.Chmod(f, 0644); err != nil {
			log.Fatalf("failed to change file permission: %s: %v", f, err)
		}
	}

	if verbose {
		log.Printf("files written: %s", strings.Join(files, ", "))
	}
}

func injectAtomStylesheet(atom string, style string) string {
	preamble := `<?xml version="1.0" encoding="UTF-8"?>`
	stylesheet := fmt.Sprintf(`<?xml-stylesheet href="%s" type="text/xsl"?>`, style)

	return strings.Replace(atom, preamble, fmt.Sprintf("%s\n%s\n", preamble, stylesheet), 1)
}

func adjustAtomLinks(atom string, file string) string {
	re := regexp.MustCompile(`(?m)^(\s*<link href="[^"]+)"></link>`)

	return re.ReplaceAllString(atom, `${1}`+file+`" rel="self"/>`+"\n"+`${1}" rel="alternate"/>`)
}

func adjustRssAuthors(rss string) string {
	dcre := regexp.MustCompile(`(<rss [^>]+)>`)
	re := regexp.MustCompile(`<author>(.*?)</author>`)

	rss = dcre.ReplaceAllString(rss, "\n"+`$1 xmlns:dc="http://purl.org/dc/elements/1.1/">`)

	return re.ReplaceAllString(rss, `<dc:creator>$1</dc:creator>`)
}
