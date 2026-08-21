package internal

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// A message that fails forever used to be requeued unconditionally, pinning a
// consumer on it. Attempt counting is what bounds that, so it has to be right
// on both queue types.
func TestDeliveryAttempts(t *testing.T) {
	cases := []struct {
		name     string
		delivery amqp.Delivery
		want     int
	}{
		{
			name:     "first delivery on a classic queue",
			delivery: amqp.Delivery{Redelivered: false},
			want:     1,
		},
		{
			name:     "redelivered on a classic queue counts as the second",
			delivery: amqp.Delivery{Redelivered: true},
			want:     2,
		},
		{
			name:     "quorum queue header int32",
			delivery: amqp.Delivery{Headers: amqp.Table{"x-delivery-count": int32(3)}},
			want:     4,
		},
		{
			name:     "quorum queue header int64",
			delivery: amqp.Delivery{Headers: amqp.Table{"x-delivery-count": int64(7)}},
			want:     8,
		},
		{
			name:     "quorum queue header int",
			delivery: amqp.Delivery{Headers: amqp.Table{"x-delivery-count": 2}},
			want:     3,
		},
		{
			// An unexpected header type must not be read as zero attempts, or a
			// poison message would retry forever again.
			name:     "unusable header falls back to the redelivered flag",
			delivery: amqp.Delivery{Headers: amqp.Table{"x-delivery-count": "3"}, Redelivered: true},
			want:     2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deliveryAttempts(tc.delivery); got != tc.want {
				t.Errorf("deliveryAttempts() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDeliveryAttemptsReachesTheCap(t *testing.T) {
	exhausted := amqp.Delivery{
		Headers: amqp.Table{"x-delivery-count": int32(maxDeliveryAttempts)},
	}

	if got := deliveryAttempts(exhausted); got < maxDeliveryAttempts {
		t.Errorf("deliveryAttempts() = %d, which would never trigger dead-lettering at %d",
			got, maxDeliveryAttempts)
	}
}
