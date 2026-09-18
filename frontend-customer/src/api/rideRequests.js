import { apiRequest } from "./client";

export async function getServiceCategories({ signal } = {}) {
  const response = await apiRequest("/service-categories", {
    authenticated: true,
    signal,
  });

  return response.data ?? [];
}

export async function createRideRequest(
  {
    pickupAddress,
    pickupLatitude,
    pickupLongitude,
    destinationAddress,
    destinationLatitude,
    destinationLongitude,
    serviceCategoryId,
    passengerCount,
    notes,
  },
  { signal } = {},
) {
  const response = await apiRequest("/ride-requests", {
    method: "POST",
    authenticated: true,
    signal,
    body: {
      pickup_address: pickupAddress,
      pickup_latitude: pickupLatitude,
      pickup_longitude: pickupLongitude,
      destination_address: destinationAddress,
      destination_latitude: destinationLatitude,
      destination_longitude: destinationLongitude,
      service_category_id: serviceCategoryId,
      passenger_count: passengerCount,
      notes,
    },
  });

  return response.data;
}
