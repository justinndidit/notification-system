package internal

import (
	"testing"
	"time"
)

func TestCalculateDeliveryStatusCountsPerToken(t *testing.T) {
	msg := &PushNotificationMessage{
		NotificationID: "n1",
		Tokens: []DeviceToken{
			{Token: "a", Platform: PlatformAndroid},
			{Token: "b", Platform: PlatformAndroid},
			{Token: "c", Platform: PlatformIOS},
		},
	}

	results := []PushResult{
		{Token: "a", Success: true, SentAt: time.Now()},
		{Token: "b", Success: false, Error: "unregistered", SentAt: time.Now()},
		{Token: "c", Success: false, Error: "iOS push not implemented", SentAt: time.Now()},
	}

	s := &PushService{}
	status := s.calculateDeliveryStatus(msg, results)

	if status.TotalTokens != 3 {
		t.Errorf("TotalTokens = %d, want 3", status.TotalTokens)
	}
	if status.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", status.SuccessCount)
	}
	if status.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", status.FailureCount)
	}
	if status.NotificationID != "n1" {
		t.Errorf("NotificationID = %q, want %q", status.NotificationID, "n1")
	}
}

func TestCalculateDeliveryStatusWithNoResults(t *testing.T) {
	// A message whose tokens all failed to send must not report as a success.
	msg := &PushNotificationMessage{
		NotificationID: "n2",
		Tokens:         []DeviceToken{{Token: "a", Platform: PlatformAndroid}},
	}

	s := &PushService{}
	status := s.calculateDeliveryStatus(msg, nil)

	if status.SuccessCount != 0 {
		t.Errorf("SuccessCount = %d, want 0", status.SuccessCount)
	}
	if status.TotalTokens != 1 {
		t.Errorf("TotalTokens = %d, want 1", status.TotalTokens)
	}
}
