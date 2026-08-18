package captcha

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/database"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCaptchaGenerateAndVerify(t *testing.T) {
	m := New(newTestDB(t))

	id, b64, err := m.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if id == "" || b64 == "" {
		t.Fatal("Generate returned empty id or image")
	}

	// A wrong answer fails.
	if m.Verify(id, "definitely-wrong-00000") {
		t.Fatal("Verify must reject a wrong answer")
	}

	if m.Verify("", "") {
		t.Fatal("Verify must reject empty id/answer")
	}
	if m.Verify(id, "") {
		t.Fatal("Verify must reject empty answer")
	}
}

// TestCaptchaCrossInstance proves a code generated on one instance's store is
// verifiable on another's, then cleared one-shot.
func TestCaptchaCrossInstance(t *testing.T) {
	db := newTestDB(t)
	store1 := &dbStore{db: db}
	store2 := &dbStore{db: db}

	if err := store1.Set("id", "12345"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !store2.Verify("id", "12345", true) {
		t.Fatal("instance B must verify a code instance A generated")
	}
	if store2.Verify("id", "12345", true) {
		t.Fatal("code must be one-shot (cleared after verify)")
	}
}
