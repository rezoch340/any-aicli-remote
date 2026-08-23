package provider

import "testing"

func validHistoryPolicy() HistoryPolicy {
	return HistoryPolicy{DefaultLimit: 10, LiveLimit: 20, MinLimit: 5, MaxLimit: 30, DefaultMaxBytes: 100, LiveMaxBytes: 200, BeforeMaxBytes: 300, MinMaxBytes: 80, MaxMaxBytes: 400, AdapterEventLimit: 40, AdapterReadBytes: 500, TitleBatchLimit: 6, ChatTextMaxRunes: 100, MessageScanInitialBytes: 10, MessageScanMaxBytes: 1000, MetadataTitleMaxRunes: 80, MetadataSummaryMaxRunes: 160, RenameTitleMaxRunes: 160}
}
func TestHistoryPolicyValidateAndNormalize(testContext *testing.T) {
	policy := validHistoryPolicy()
	if operationError := policy.Validate(); operationError != nil {
		testContext.Fatal(operationError)
	}
	limit, maximum := policy.NormalizeRequest(true, nil, 0, 0)
	if limit != 20 || maximum != 200 {
		testContext.Fatalf("got %d %d", limit, maximum)
	}
	limit, maximum = policy.NormalizeRequest(false, new(int64), 1, 999)
	if limit != 5 || maximum != 400 {
		testContext.Fatalf("clamp got %d %d", limit, maximum)
	}
	policy.MinLimit = 21
	if policy.Validate() == nil {
		testContext.Fatal("expected invalid policy")
	}
}
