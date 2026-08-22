package room

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSayFeedMembersClear(testContext *testing.T) {
	store := New(testContext.TempDir())
	base := time.Unix(1_700_000_000, 0)
	tick := 0
	store.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	}

	if got := store.Say("  alpha  bot  ", " hello\n\tworld ", " say-more-than-twelve "); !got.OK {
		testContext.Fatalf("Say failed: %+v", got)
	} else {
		if got.Message.ID != 1 || got.Message.Who != "alpha bot" || got.Message.Text != "hello world" || got.Message.Kind != "say-more-tha" {
			testContext.Fatalf("unexpected message: %+v", got.Message)
		}
	}
	_ = store.Say("beta", "second", "say")
	_ = store.Say("alpha bot", "third", "say")

	feed, operationError := store.Feed(1, 10)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(feed) != 2 || feed[0].Text != "second" || feed[1].Text != "third" {
		testContext.Fatalf("bad feed: %#v", feed)
	}
	one, operationError := store.Feed(0, 1)
	if operationError != nil || len(one) != 1 || one[0].ID != 3 {
		testContext.Fatalf("bad limited feed %#v err=%v", one, operationError)
	}

	members, operationError := store.Members(900 * time.Second)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(members) != 2 || members[0].Who != "alpha bot" || members[0].Count != 2 || members[1].Who != "beta" {
		testContext.Fatalf("bad members: %#v", members)
	}

	if operationError := store.Clear(); operationError != nil {
		testContext.Fatal(operationError)
	}
	cleared, operationError := store.Feed(0, 10)
	if operationError != nil || len(cleared) != 0 {
		testContext.Fatalf("clear failed feed=%#v err=%v", cleared, operationError)
	}
}

func TestCleanRuneLimitAndDataDirectory(testContext *testing.T) {
	if got := Clean(strings.Repeat("界", Limit+5)); len([]rune(got)) != Limit {
		testContext.Fatalf("Clean rune cap = %d", len([]rune(got)))
	}
	directory := testContext.TempDir()
	testContext.Setenv("GROK_PLUGIN_DATA", filepath.Join(directory, "data"))
	got, operationError := DataDirectory()
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := os.Stat(got); operationError != nil {
		testContext.Fatal(operationError)
	}
	if got != filepath.Join(directory, "data") {
		testContext.Fatalf("DataDir=%q", got)
	}
}

func TestCompactionKeepsIDs(testContext *testing.T) {
	store := New(testContext.TempDir())
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	for itemIndex := 0; itemIndex < Keep+1; itemIndex++ {
		if result := store.Say("agent", "msg", "say"); !result.OK {
			testContext.Fatalf("say %d: %+v", itemIndex, result)
		}
	}
	store.mutex.Lock()
	messages, operationError := store.readAllLocked()
	store.mutex.Unlock()
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(messages) != Keep/2+1 {
		testContext.Fatalf("compacted len=%d want %d", len(messages), Keep/2+1)
	}
	if messages[len(messages)-1].ID != Keep+1 {
		testContext.Fatalf("last id=%d", messages[len(messages)-1].ID)
	}
}
