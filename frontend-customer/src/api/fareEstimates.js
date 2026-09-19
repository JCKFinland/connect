import { apiRequest } from "./client";

export async function getFareEstimate(
  {
    pickupLatitude,
    pickupLongitude,
    destinationLatitude,
    destinationLongitude,
    serviceCategoryId,
  },
  { signal } = {},
) {
  const response = await apiRequest("/fare-estimates", {
    method: "POST",
    authenticated: true,
    signal,
    body: {
      pickup_latitude: pickupLatitude,
      pickup_longitude: pickupLongitude,
      destination_latitude: destinationLatitude,
      destination_longitude: destinationLongitude,
      service_category_id: serviceCategoryId,
    },
  });

  return response.data;
}