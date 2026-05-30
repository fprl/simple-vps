package releaseid

import (
	"testing"
	"time"
)

func TestDirtyIncludesNanosecondTimestamp(t *testing.T) {
	at := time.Date(2026, 5, 30, 14, 30, 12, 123456789, time.UTC)
	got := Dirty("a1b2c3d4e5f6", at)
	want := "a1b2c3d4e5f6-dirty-20260530t143012123456789z"
	if got != want {
		t.Fatalf("Dirty = %q, want %q", got, want)
	}
}

func TestParseReleaseIDs(t *testing.T) {
	info, err := Parse("a1b2c3d4e5f6-dirty-20260530t143012123456789z-s012345abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Dirty || info.Base != "a1b2c3d4e5f6" || info.Timestamp != "20260530t143012123456789z" || info.StaticHash != "012345abcdef" {
		t.Fatalf("unexpected parse info: %+v", info)
	}

	info, err = Parse("a1b2c3d4e5f6-s012345abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if info.Dirty || info.Base != "a1b2c3d4e5f6" || info.StaticHash != "012345abcdef" {
		t.Fatalf("unexpected clean parse info: %+v", info)
	}
}

func TestRejectsOldSecondPrecisionDirtyIDs(t *testing.T) {
	if err := Validate("a1b2c3d4e5f6-dirty-20260530t143012z"); err == nil {
		t.Fatal("expected old second-precision dirty id to be rejected")
	}
}

func TestWithStaticHash(t *testing.T) {
	got, err := WithStaticHash("a1b2c3d4e5f6", "012345abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a1b2c3d4e5f6-s012345abcdef" {
		t.Fatalf("WithStaticHash = %q", got)
	}
	if _, err := WithStaticHash(got, "012345abcdef"); err == nil {
		t.Fatal("expected double static hash to fail")
	}
}
