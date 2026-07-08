package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	twitterscraper "github.com/jeffrey12cali/twitter-scraper"
)

func sampleTweet(id, text string) *twitterscraper.Tweet {
	return &twitterscraper.Tweet{
		ID:        id,
		Text:      text,
		Username:  "bob",
		Name:      "Bob Smith",
		Timestamp: 1609459200, // 2021-01-01 UTC
		Likes:     10,
		Retweets:  3,
		Replies:   1,
		Views:     100,
		Hashtags:  []string{"go", "twitter"},
		Mentions: []twitterscraper.Mention{
			{Username: "alice"},
			{Username: "carol"},
		},
		URLs:           []string{"https://example.com"},
		PermanentURL:   "https://x.com/bob/status/" + id,
		IsRetweet:      false,
		IsReply:        true,
		QuotedStatusID: "999",
		ConversationID: "111",
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}

func TestBuildMeta(t *testing.T) {
	m := buildMeta(sampleTweet("123", "hello"))
	if m.ID != "123" || m.Username != "bob" || m.Text != "hello" {
		t.Fatalf("basic fields wrong: %+v", m)
	}
	want := time.Unix(1609459200, 0).Format(datefmt)
	if m.Date != want {
		t.Fatalf("date = %q, want %q", m.Date, want)
	}
	if len(m.Mentions) != 2 || m.Mentions[0] != "alice" || m.Mentions[1] != "carol" {
		t.Fatalf("mentions not mapped to usernames: %v", m.Mentions)
	}
	if m.Likes != 10 || m.Retweets != 3 || m.Replies != 1 || m.Views != 100 {
		t.Fatalf("counts wrong: %+v", m)
	}
	if m.QuotedStatusID != "999" || m.ConversationID != "111" || !m.IsReply {
		t.Fatalf("flags/ids wrong: %+v", m)
	}
}

func TestAppendText_FormatAndAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.txt")
	if wrote, err := appendText(path, sampleTweet("1", "first tweet"), nil); err != nil || !wrote {
		t.Fatalf("A: wrote=%v err=%v", wrote, err)
	}
	if wrote, err := appendText(path, sampleTweet("2", "second tweet"), nil); err != nil || !wrote {
		t.Fatalf("B: wrote=%v err=%v", wrote, err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "=== 1 | ") || !strings.Contains(s, "@bob") {
		t.Fatalf("missing header: %q", s)
	}
	if !strings.Contains(s, "first tweet") || !strings.Contains(s, "second tweet") {
		t.Fatalf("missing body: %q", s)
	}
	if strings.Count(s, "=== ") != 2 {
		t.Fatalf("want 2 blocks, got %d", strings.Count(s, "=== "))
	}
}

func TestAppendText_SkipsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.txt")
	wrote, err := appendText(path, sampleTweet("1", ""), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if wrote {
		t.Fatal("empty text should be skipped")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("no file should be created for empty text")
	}
}

func TestAppendMeta_JSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.jsonl")
	appendMeta(path, sampleTweet("1", "a"), nil)
	appendMeta(path, sampleTweet("2", "b"), nil)
	if n := countLines(t, path); n != 2 {
		t.Fatalf("want 2 lines, got %d", n)
	}
	f, _ := os.Open(path)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan()
	var m tweetMeta
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
		t.Fatalf("line not valid json: %v", err)
	}
	if m.ID != "1" || m.Text != "a" || len(m.Mentions) != 2 {
		t.Fatalf("unmarshalled wrong: %+v", m)
	}
}

func TestLoadSeenIDs_Jsonl(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.jsonl")
	appendMeta(path, sampleTweet("10", "x"), nil)
	appendMeta(path, sampleTweet("20", "y"), nil)
	seen := loadSeenIDs(path, "jsonl")
	if !seen["10"] || !seen["20"] || len(seen) != 2 {
		t.Fatalf("seen = %v", seen)
	}
}

func TestLoadSeenIDs_Txt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.txt")
	appendText(path, sampleTweet("10", "x"), nil)
	appendText(path, sampleTweet("20", "y"), nil)
	seen := loadSeenIDs(path, "txt")
	if !seen["10"] || !seen["20"] || len(seen) != 2 {
		t.Fatalf("seen = %v", seen)
	}
}

func TestLoadSeenIDs_MissingFile(t *testing.T) {
	seen := loadSeenIDs(filepath.Join(t.TempDir(), "nope.jsonl"), "jsonl")
	if len(seen) != 0 {
		t.Fatalf("missing file should give empty set, got %v", seen)
	}
}

func TestUpdateDedup_Text(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.txt")
	appendText(path, sampleTweet("A", "old"), nil) // pre-seed
	seen := loadSeenIDs(path, "txt")

	if wrote, _ := appendText(path, sampleTweet("A", "old"), seen); wrote {
		t.Fatal("existing id A should be skipped under -U")
	}
	if wrote, _ := appendText(path, sampleTweet("B", "new"), seen); !wrote {
		t.Fatal("new id B should be written")
	}
	data, _ := os.ReadFile(path)
	if c := strings.Count(string(data), "=== A |"); c != 1 {
		t.Fatalf("A appears %d times, want 1", c)
	}
	if c := strings.Count(string(data), "=== B |"); c != 1 {
		t.Fatalf("B appears %d times, want 1", c)
	}
}

func TestUpdateDedup_Meta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tweets.jsonl")
	appendMeta(path, sampleTweet("A", "old"), nil) // pre-seed
	seen := loadSeenIDs(path, "jsonl")

	if wrote, _ := appendMeta(path, sampleTweet("A", "old"), seen); wrote {
		t.Fatal("existing id A should be skipped under -U")
	}
	if wrote, _ := appendMeta(path, sampleTweet("B", "new"), seen); !wrote {
		t.Fatal("new id B should be written")
	}
	if n := countLines(t, path); n != 2 {
		t.Fatalf("want 2 lines (A once + B once), got %d", n)
	}
}

func TestDedup_InRun(t *testing.T) {
	seen := map[string]bool{}
	pathTxt := filepath.Join(t.TempDir(), "tweets.txt")
	appendText(pathTxt, sampleTweet("A", "x"), seen)
	if wrote, _ := appendText(pathTxt, sampleTweet("A", "x"), seen); wrote {
		t.Fatal("second write of same id in-run should be skipped")
	}
	if c := countLines(t, pathTxt); c == 0 {
		t.Fatal("first write missing")
	}
}
