package dtos

import (
	"testing"
)

// pgx hands JSONB back as []byte or string depending on the codec in play.
// Asserting only []byte made every repository read fail at scan time, and the
// failure was invisible until something actually called those methods.
func TestUserDataScanAcceptsBothPgxRepresentations(t *testing.T) {
	const raw = `{"name":"Ada","link":"https://example.com","meta":{"plan":"pro"}}`

	cases := []struct {
		name  string
		value any
	}{
		{"bytes", []byte(raw)},
		{"string", raw},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got UserData
			if err := got.Scan(tc.value); err != nil {
				t.Fatalf("Scan(%T) returned error: %v", tc.value, err)
			}

			if got.Name != "Ada" {
				t.Errorf("Name = %q, want %q", got.Name, "Ada")
			}
			if got.Link != "https://example.com" {
				t.Errorf("Link = %q, want %q", got.Link, "https://example.com")
			}
			if got.Meta["plan"] != "pro" {
				t.Errorf("Meta[plan] = %v, want %q", got.Meta["plan"], "pro")
			}
		})
	}
}

func TestUserDataScanNilLeavesZeroValue(t *testing.T) {
	var got UserData
	if err := got.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) returned error: %v", err)
	}
	if got.Name != "" || got.Link != "" {
		t.Errorf("Scan(nil) mutated the value: %+v", got)
	}
}

func TestUserDataScanRejectsUnsupportedType(t *testing.T) {
	var got UserData
	if err := got.Scan(42); err == nil {
		t.Fatal("Scan(int) should return an error")
	}
}

func TestUserDataRoundTrip(t *testing.T) {
	original := UserData{
		Name: "Grace",
		Link: "https://example.com/x",
		Meta: map[string]any{"tier": "gold"},
	}

	encoded, err := original.Value()
	if err != nil {
		t.Fatalf("Value() returned error: %v", err)
	}

	var decoded UserData
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	if decoded.Name != original.Name || decoded.Link != original.Link {
		t.Errorf("round trip changed the value: %+v -> %+v", original, decoded)
	}
}

func TestNotificationPriorityToString(t *testing.T) {
	cases := map[NotificationPriority]string{
		Low:    "low",
		Normal: "normal",
		High:   "high",
		Urgent: "urgent",
		// Anything unrecognised must fall back rather than produce an empty
		// string: priority is CHECK-constrained in the database.
		NotificationPriority(99): "normal",
		NotificationPriority(0):  "normal",
	}

	for input, want := range cases {
		if got := NotificationPriorityToString(input); got != want {
			t.Errorf("NotificationPriorityToString(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestNotificationTypeScan(t *testing.T) {
	var channel NotificationType
	if err := channel.Scan("email"); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if channel != Email {
		t.Errorf("channel = %q, want %q", channel, Email)
	}

	if err := channel.Scan(123); err == nil {
		t.Error("Scan(int) should return an error")
	}
}
