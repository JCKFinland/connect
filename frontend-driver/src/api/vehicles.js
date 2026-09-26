import { apiRequest } from "./client";

export function getDriverFleetsRequest({ signal } = {}) {
  return apiRequest("/driver/fleets", {
    authenticated: true,
    signal,
  });
}

export function registerDriverVehicleRequest({
  fleetId,
  registrationNumber,
  vin,
  make,
  model,
  modelYear,
  color,
  vehicleType,
  fuelType,
  seatingCapacity,
}) {
  return apiRequest("/driver/vehicles", {
    method: "POST",
    authenticated: true,
    body: {
      fleet_id: fleetId,
      registration_number: registrationNumber,
      vin,
      make,
      model,
      model_year: modelYear,
      color,
      vehicle_type: vehicleType,
      fuel_type: fuelType,
      seating_capacity: seatingCapacity,
    },
  });
}
