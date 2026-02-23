package database_test

import (
	"testing"

	"github.com/clawbake/clawbake/internal/database"
)

func TestNewQueries(t *testing.T) {
	q := database.New(nil)
	if q == nil {
		t.Fatal("expected non-nil Queries")
	}
}
