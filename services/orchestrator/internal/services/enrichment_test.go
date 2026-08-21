package services

import (
	"strings"
	"testing"
	"time"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
)

func TestChannelAllowedHonoursConsent(t *testing.T) {
	cases := []struct {
		name    string
		channel dtos.NotificationType
		profile dtos.UserDeliveryProfile
		want    bool
	}{
		{
			name:    "email opted in",
			channel: dtos.Email,
			profile: dtos.UserDeliveryProfile{EmailOptIn: true, PushOptIn: false},
			want:    true,
		},
		{
			name:    "email opted out",
			channel: dtos.Email,
			profile: dtos.UserDeliveryProfile{EmailOptIn: false, PushOptIn: true},
			want:    false,
		},
		{
			name:    "push opted in",
			channel: dtos.Push,
			profile: dtos.UserDeliveryProfile{EmailOptIn: false, PushOptIn: true},
			want:    true,
		},
		{
			name:    "push opted out",
			channel: dtos.Push,
			profile: dtos.UserDeliveryProfile{EmailOptIn: true, PushOptIn: false},
			want:    false,
		},
		{
			// A channel nobody has consented to must not default to allowed.
			name:    "unknown channel is denied",
			channel: dtos.NotificationType("carrier-pigeon"),
			profile: dtos.UserDeliveryProfile{EmailOptIn: true, PushOptIn: true},
			want:    false,
		},
	}

	o := &Orchestrator{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := o.channelAllowed(tc.channel, tc.profile); got != tc.want {
				t.Errorf("channelAllowed(%q) = %v, want %v", tc.channel, got, tc.want)
			}
		})
	}
}

func TestResolveRecipient(t *testing.T) {
	withEmail := dtos.UserDeliveryProfile{UserID: "u1", Email: "ada@example.com"}
	withTokens := dtos.UserDeliveryProfile{
		UserID:       "u2",
		DeviceTokens: []dtos.DeviceToken{{Token: "fcm-abc", Platform: "android"}},
	}

	o := &Orchestrator{}

	t.Run("email uses the address", func(t *testing.T) {
		got, err := o.resolveRecipient(dtos.Email, withEmail)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ada@example.com" {
			t.Errorf("recipient = %q, want %q", got, "ada@example.com")
		}
	})

	t.Run("push uses a device token", func(t *testing.T) {
		got, err := o.resolveRecipient(dtos.Push, withTokens)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "fcm-abc" {
			t.Errorf("recipient = %q, want %q", got, "fcm-abc")
		}
	})

	// Publishing a message no worker can deliver is worse than failing here:
	// the worker would drop it and the notification would look successful.
	t.Run("email without an address fails", func(t *testing.T) {
		if _, err := o.resolveRecipient(dtos.Email, dtos.UserDeliveryProfile{UserID: "u3"}); err == nil {
			t.Fatal("expected an error when the user has no email")
		}
	})

	t.Run("push without tokens fails", func(t *testing.T) {
		if _, err := o.resolveRecipient(dtos.Push, dtos.UserDeliveryProfile{UserID: "u4"}); err == nil {
			t.Fatal("expected an error when the user has no device tokens")
		}
	})

	t.Run("unsupported channel fails", func(t *testing.T) {
		_, err := o.resolveRecipient(dtos.NotificationType("sms"), withEmail)
		if err == nil {
			t.Fatal("expected an error for an unsupported channel")
		}
		if !strings.Contains(err.Error(), "sms") {
			t.Errorf("error should name the channel, got %q", err)
		}
	})
}

func TestRenderedBodyPrefersHTML(t *testing.T) {
	cases := []struct {
		name    string
		content dtos.RenderedContent
		want    string
	}{
		{
			name:    "email html wins",
			content: dtos.RenderedContent{HTML: "<p>hi</p>", Body: "hi"},
			want:    "<p>hi</p>",
		},
		{
			name:    "push falls back to body",
			content: dtos.RenderedContent{Body: "hi"},
			want:    "hi",
		},
		{
			name:    "empty stays empty",
			content: dtos.RenderedContent{},
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderedBody(tc.content); got != tc.want {
				t.Errorf("renderedBody() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToJSONMapRoundTripsTheQueuedMessage(t *testing.T) {
	// Retry republishes whatever was stored, so the stored form must survive a
	// round trip through JSONB without losing fields.
	original := dtos.EnrichedNotification{
		NotificationID: "n1",
		CorrelationID:  "c1",
		Channel:        "email",
		Recipient:      "ada@example.com",
		Subject:        "Welcome",
		Body:           "<p>Hello</p>",
		Tokens:         []dtos.DeviceToken{{Token: "t", Platform: "ios"}},
		CreatedAt:      time.Now().UTC().Truncate(time.Second),
	}

	stored, err := toJSONMap(original)
	if err != nil {
		t.Fatalf("toJSONMap returned error: %v", err)
	}

	if stored["recipient"] != "ada@example.com" {
		t.Errorf("recipient lost in storage: %v", stored["recipient"])
	}
	if stored["subject"] != "Welcome" {
		t.Errorf("subject lost in storage: %v", stored["subject"])
	}
}

func TestPriorityFromStringInvertsToString(t *testing.T) {
	// The sweeper reconstructs a request from the stored row, so these two must
	// stay inverse or a recovered notification changes priority.
	for _, p := range []dtos.NotificationPriority{dtos.Low, dtos.Normal, dtos.High, dtos.Urgent} {
		asString := dtos.NotificationPriorityToString(p)
		back := priorityFromString(asString)

		if back != int(p) {
			t.Errorf("round trip for %q: got %d, want %d", asString, back, int(p))
		}
	}

	if got := priorityFromString("nonsense"); got != int(dtos.Normal) {
		t.Errorf("unknown priority = %d, want %d", got, int(dtos.Normal))
	}
}

func TestMapCallbackStatus(t *testing.T) {
	cases := []struct {
		reported  string
		status    string
		eventType string
		ok        bool
	}{
		{"sent", dtos.StatusSent, dtos.EventSent, true},
		{"delivered", dtos.StatusSent, dtos.EventDelivered, true},
		{"failed", dtos.StatusFailed, dtos.EventFailed, true},
		{"bounced", dtos.StatusFailed, dtos.EventBounced, true},
		// An unrecognised status must be rejected, not silently stored: status
		// is CHECK-constrained in the database.
		{"exploded", "", "", false},
		{"", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.reported, func(t *testing.T) {
			status, eventType, ok := mapCallbackStatus(tc.reported)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if status != tc.status || eventType != tc.eventType {
				t.Errorf("got (%q, %q), want (%q, %q)", status, eventType, tc.status, tc.eventType)
			}
		})
	}
}

func TestOutboxBackoffGrowsAndIsCapped(t *testing.T) {
	first := outboxBackoff(0)
	second := outboxBackoff(1)

	if first != outboxBaseBackoff {
		t.Errorf("first backoff = %v, want %v", first, outboxBaseBackoff)
	}
	if second <= first {
		t.Errorf("backoff should grow: %v then %v", first, second)
	}

	// A long-failing entry must not schedule itself decades out, and integer
	// overflow must not wrap it back to something tiny.
	for _, attempts := range []int{20, 100, 1000} {
		if got := outboxBackoff(attempts); got != outboxMaxBackoff {
			t.Errorf("outboxBackoff(%d) = %v, want the cap %v", attempts, got, outboxMaxBackoff)
		}
	}

	if got := outboxBackoff(-1); got != outboxBaseBackoff {
		t.Errorf("outboxBackoff(-1) = %v, want %v", got, outboxBaseBackoff)
	}
}
