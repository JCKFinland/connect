package ride_request

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin/binding"
)

func TestCreateRideRequestRequestAllowsMissingCustomerID(t *testing.T) {
	body := []byte(`{
		"pickup_address": "Espoo Test Pickup",
		"pickup_latitude": 60.2055,
		"pickup_longitude": 24.6559,
		"destination_address": "Helsinki Central Station",
		"destination_latitude": 60.1719,
		"destination_longitude": 24.9414,
		"service_category_id": "11111111-1111-4111-8111-111111111111",
		"passenger_count": 1
	}`)

	var req CreateRideRequestRequest

	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if err := binding.Validator.ValidateStruct(req); err != nil {
		t.Fatalf(
			"expected request without customer_id to pass binding validation, got %v",
			err,
		)
	}

	if req.CustomerID != "" {
		t.Fatalf(
			"expected omitted customer_id to remain empty before authorization, got %q",
			req.CustomerID,
		)
	}
}
