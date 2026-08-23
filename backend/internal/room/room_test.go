package room

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSayFeedMembersClear(testContext *testing.T) {
	store := newTestStore(testContext)
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

	members, operationError := store.Members()
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

func TestCleanRuneLimitAndPolicy(testContext *testing.T) {
	policy := testPolicy()
	if got := clean(strings.Repeat("界", policy.MessageRuneLimit+5), policy.MessageRuneLimit); len([]rune(got)) != policy.MessageRuneLimit {
		testContext.Fatalf("clean rune cap = %d", len([]rune(got)))
	}
}

func TestCompactionKeepsIDs(testContext *testing.T) {
	store := newTestStore(testContext)
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	for itemIndex := 0; itemIndex < store.policy.CompactionThreshold+1; itemIndex++ {
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
	if len(messages) != store.policy.CompactionRetainMessages+1 {
		testContext.Fatalf("compacted len=%d want %d", len(messages), store.policy.CompactionRetainMessages+1)
	}
	if messages[len(messages)-1].ID != store.policy.CompactionThreshold+1 {
		testContext.Fatalf("last id=%d", messages[len(messages)-1].ID)
	}
}

func testPolicy() Policy {
	return Policy{MessageRuneLimit: 240, SpeakerRuneLimit: 32, KindRuneLimit: 12, CompactionThreshold: 20, CompactionRetainMessages: 10, FeedDefaultLimit: 4, FeedMaxLimit: 8, MemberWindow: 15 * time.Minute, ScannerInitialBytes: 64 * 1024, ScannerMaxBytes: 4 * 1024 * 1024}
}
func newTestStore(testContext *testing.T) *Store {
	testContext.Helper()
	store, operationError := New(testContext.TempDir(), testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return store
}

func TestPolicyControlsRoomBehavior(testContext *testing.T) {
	policy := Policy{MessageRuneLimit: 3, SpeakerRuneLimit: 2, KindRuneLimit: 1, CompactionThreshold: 4, CompactionRetainMessages: 2, FeedDefaultLimit: 2, FeedMaxLimit: 3, MemberWindow: time.Minute, ScannerInitialBytes: 8, ScannerMaxBytes: 512}
	store, operationError := New(testContext.TempDir(), policy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	now := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return now }
	if result := store.Say("界界界", "界界界界", "long"); !result.OK || result.Message.Who != "界界" || result.Message.Text != "界界界" || result.Message.Kind != "l" {
		testContext.Fatalf("policy truncation = %#v", result)
	}
	for index := 0; index < 4; index++ {
		if !store.Say("recent", "msg", "say").OK {
			testContext.Fatal("say failed")
		}
	}
	feed, operationError := store.FeedString("0", "invalid")
	if operationError != nil || len(feed) != 2 {
		testContext.Fatalf("default feed = %#v %v", feed, operationError)
	}
	feed, operationError = store.Feed(0, 99)
	if operationError != nil || len(feed) != 3 {
		testContext.Fatalf("max feed = %#v %v", feed, operationError)
	}
	old := Message{ID: 99, Timestamp: float64(now.Add(-2 * time.Minute).Unix()), Who: "old", Text: "old", Kind: "s"}
	path, _ := store.Path()
	encoded, _ := json.Marshal(old)
	if operationError := os.WriteFile(path, append(encoded, '\n'), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	members, operationError := store.Members()
	if operationError != nil || len(members) != 0 {
		testContext.Fatalf("member window = %#v %v", members, operationError)
	}
	if operationError := os.WriteFile(path, []byte(strings.Repeat("x", 513)), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := store.Feed(0, 1); operationError == nil {
		testContext.Fatal("scanner maximum accepted oversized line")
	}
}
func TestNewRejectsMissingDirectoryAndInvalidPolicy(testContext *testing.T) {
	if _, operationError := New("", testPolicy()); operationError == nil {
		testContext.Fatal("accepted empty directory")
	}
	if _, operationError := New(testContext.TempDir(), Policy{}); operationError == nil {
		testContext.Fatal("accepted zero policy")
	}
}
