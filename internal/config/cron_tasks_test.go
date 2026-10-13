package config

import (
	"testing"
)

func TestValidateCronExpr(t *testing.T) {
	valid := []string{
		"0 */1 * * *",
		"*/1 * * * *",
		"*/5 * * * *",
		"0 0 * * *",
		"15 10 * * *",
		"0,15,30,45 * * * *",
		"0 9-17 * * 1-5",
		"0 0 1 1 *",
		"*/30 * * * * *", // 6 fields (with seconds)
		"0 0 12 * * ?",
	}

	for _, expr := range valid {
		if err := ValidateCronExpr(expr); err != nil {
			t.Errorf("expected valid for %q, got error: %v", expr, err)
		}
	}

	invalid := []string{
		"",
		"* * *",
		"* * * * * * *", // 7 fields
		"60 * * * *",    // min 60 out of bounds
		"* 25 * * *",    // hour 25 out of bounds
		"* * 32 * *",    // dom 32 out of bounds
		"* * * 13 *",    // mon 13 out of bounds
		"* * * * 8",     // dow 8 out of bounds
		"*/0 * * * *",   // step 0 invalid
		"abc * * * *",
	}

	for _, expr := range invalid {
		if err := ValidateCronExpr(expr); err == nil {
			t.Errorf("expected error for invalid expression %q, but got nil", expr)
		}
	}
}
