package voice

import (
	"math"
	"testing"
	"time"
)

func TestPolicyValidate(testContext *testing.T) {
	valid := Policy{RequestTimeout: time.Second, TextMaxRunes: 10, TruncatedTextRunes: 9, SuccessBodyMaxBytes: 10, ErrorBodyMaxBytes: 10, ErrorBodyMaxRunes: 5}
	if errorValue := valid.Validate(); errorValue != nil {
		testContext.Fatal(errorValue)
	}
	if errorValue := (Policy{}).Validate(); errorValue == nil {
		testContext.Fatal("zero policy accepted")
	}
	valid.TruncatedTextRunes = valid.TextMaxRunes
	if errorValue := valid.Validate(); errorValue == nil {
		testContext.Fatal("invalid truncation relation accepted")
	}
	valid = Policy{RequestTimeout: time.Second, TextMaxRunes: math.MaxInt, TruncatedTextRunes: math.MaxInt, SuccessBodyMaxBytes: 10, ErrorBodyMaxBytes: 10, ErrorBodyMaxRunes: 5}
	if errorValue := valid.Validate(); errorValue == nil {
		testContext.Fatal("max int relation accepted")
	}
}
