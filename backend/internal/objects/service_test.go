package objects

import "testing"

func TestParseTags(t *testing.T) {
	tags, err := ParseTags("team=platform&classification=internal")
	if err != nil {
		t.Fatalf("parse tags: %v", err)
	}
	if tags["team"] != "platform" {
		t.Fatalf("expected team tag, got %#v", tags)
	}
	if tags["classification"] != "internal" {
		t.Fatalf("expected classification tag, got %#v", tags)
	}
}

func TestParseTagsAllowsEmptyInput(t *testing.T) {
	tags, err := ParseTags("")
	if err != nil {
		t.Fatalf("parse empty tags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected no tags, got %#v", tags)
	}
}

func TestNormalizeTagsDropsEmptyKeys(t *testing.T) {
	tags := NormalizeTags(map[string]string{
		"":       "ignored",
		"owner":  "storage",
		"region": "lab",
	})
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %#v", tags)
	}
	if _, ok := tags[""]; ok {
		t.Fatalf("unexpected empty tag key %#v", tags)
	}
}
