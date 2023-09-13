package main

import (
	"log"
	"fmt"
	"time"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/feeds"
)

func main() {
	path := "/home/simon/Code/awesome-veganism"

	r, err := git.PlainOpen(path)
	if err != nil {
		log.Fatalf("failed to open repository: %s: %v", path, err)
	}

	// ... retrieving the HEAD reference
	ref, err := r.Head()
	if err != nil {
		log.Fatalf("failed to get HEAD reference: %v", err)
	}

	logfile := "README.md"
	logopts := &git.LogOptions{
		From:     ref.Hash(),
		FileName: &logfile,
		Order:    git.LogOrderCommitterTime,
		//Order: git.LogOrderDFSPost,
	}

	// ... retrieves the commit history
	iter, err := r.Log(logopts)
	if err != nil {
		log.Fatalf("failed to get log: %v", err)
	}

	var commits []*object.Commit

	// ... just iterates over the commits
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)

		return nil
	})
	if err != nil {
		log.Fatalf("failed to iterate commit log: %v", err)
	}

	re, err := regexp.Compile(`\n([+-])\s*[-] \[([^\]]+)\]\(([^\)]+)\) [-] ([^\n]+)`)
	if err != nil {
		log.Fatalf("failed to compile regular expression: %v", err)
	}

	now := time.Now()
	feed := &feeds.Feed{
		Title:       "AwesomeVeganism Feed",
		Link:        &feeds.Link{Href: "https://awesome-veganism.com/"},
		Description: "A curated list of awesome resources, pointers, and tips to make veganism easy and accessible to everyone.",
		Author:      &feeds.Author{Name: "Yeah, hmm", Email: "jmoiron@jmoiron.net"},
		Created:     now,
	}

	for n := len(commits) - 1; n >= 0; n-- {
		c := commits[n]

		// skip initial commit in this project as it happens to have no relevant content
		if n == 0 {
			// use date of first commit for date of creation
			feed.Created = c.Author.When
			break
		}

		p := commits[n-1]

		log.Printf("===>> commit: %s by %s at %s: %s", p.Hash, p.Author.Name, p.Author.When, p.Message)

		patch, err := c.Patch(p)
		if err != nil {
			log.Printf("failed to get patch: %v", err)
		}

		//log.Printf("patch: %s", patch)

		matches := re.FindAllStringSubmatch(patch.String(), -1)

		//log.Printf("matches: %v", matches)

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

		log.Printf("changes: %v", changes)

		continue

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

			log.Printf("====>> %s: %s -- %s -- %s", t, m[2], m[3], m[4])

			feed.Items = append(feed.Items, &feeds.Item{
				Title:       fmt.Sprintf("%s of %s", t, m[2]),
				Link:        &feeds.Link{Href: m[3]},
				Description: m[4],
				Author:      &feeds.Author{Name: p.Author.Name, Email: "jmoiron@jmoiron.net"},
				Created:     p.Author.When,
			})

			feed.Updated = p.Author.When
		}

	}

	if feed == nil {
	}

	fmt.Println(feed.ToRss())

}
