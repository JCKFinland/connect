package realtime

import (
	"testing"
	"time"
)

func TestBrokerPublishesToSubscriber(t *testing.T) {
	broker := NewBroker()

	events, unsubscribe := broker.Subscribe("trip:123")
	defer unsubscribe()

	expected := Event{
		Type: "trip.location",
		Data: "location",
	}

	broker.Publish(
		"trip:123",
		expected,
	)

	select {
	case event := <-events:
		if event.Type != expected.Type {
			t.Fatalf(
				"expected event type %q, got %q",
				expected.Type,
				event.Type,
			)
		}

		if event.Data != expected.Data {
			t.Fatalf(
				"expected event data %v, got %v",
				expected.Data,
				event.Data,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestBrokerPublishesToMultipleSubscribers(t *testing.T) {
	broker := NewBroker()

	first, unsubscribeFirst :=
		broker.Subscribe("trip:123")
	defer unsubscribeFirst()

	second, unsubscribeSecond :=
		broker.Subscribe("trip:123")
	defer unsubscribeSecond()

	broker.Publish(
		"trip:123",
		Event{
			Type: "trip.location",
			Data: "location",
		},
	)

	for index, events := range []<-chan Event{
		first,
		second,
	} {
		select {
		case <-events:

		case <-time.After(time.Second):
			t.Fatalf(
				"subscriber %d did not receive event",
				index+1,
			)
		}
	}
}

func TestBrokerSeparatesTopics(t *testing.T) {
	broker := NewBroker()

	tripOne, unsubscribeTripOne :=
		broker.Subscribe("trip:1")
	defer unsubscribeTripOne()

	tripTwo, unsubscribeTripTwo :=
		broker.Subscribe("trip:2")
	defer unsubscribeTripTwo()

	broker.Publish(
		"trip:1",
		Event{
			Type: "trip.location",
		},
	)

	select {
	case <-tripOne:
	case <-time.After(time.Second):
		t.Fatal("trip:1 subscriber did not receive event")
	}

	select {
	case event := <-tripTwo:
		t.Fatalf(
			"trip:2 unexpectedly received event: %+v",
			event,
		)

	case <-time.After(50 * time.Millisecond):
	}
}

func TestBrokerUnsubscribeStopsDelivery(t *testing.T) {
	broker := NewBroker()

	events, unsubscribe :=
		broker.Subscribe("trip:123")

	unsubscribe()

	broker.Publish(
		"trip:123",
		Event{
			Type: "trip.location",
		},
	)

	select {
	case event := <-events:
		t.Fatalf(
			"received event after unsubscribe: %+v",
			event,
		)

	case <-time.After(50 * time.Millisecond):
	}
}

func TestBrokerUnsubscribeIsIdempotent(t *testing.T) {
	broker := NewBroker()

	_, unsubscribe :=
		broker.Subscribe("trip:123")

	unsubscribe()
	unsubscribe()
}
