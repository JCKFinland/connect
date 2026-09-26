import { useEffect, useState } from "react";

import {
  getDriverFleetsRequest,
  registerDriverVehicleRequest,
} from "../api/vehicles";

const FUEL_TYPES = ["PETROL", "DIESEL", "HYBRID", "EV"];

const VEHICLE_TYPES = ["SEDAN", "WAGON", "SUV", "VAN"];

export function VehicleRegistrationPage() {
  const [fleets, setFleets] = useState([]);

  const [fleetId, setFleetId] = useState("");
  const [registrationNumber, setRegistrationNumber] = useState("");
  const [vin, setVin] = useState("");
  const [make, setMake] = useState("");
  const [model, setModel] = useState("");
  const [modelYear, setModelYear] = useState("");
  const [color, setColor] = useState("");
  const [vehicleType, setVehicleType] = useState("");
  const [fuelType, setFuelType] = useState("");
  const [seatingCapacity, setSeatingCapacity] = useState("");

  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [registeredVehicle, setRegisteredVehicle] = useState(null);

  useEffect(() => {
    const controller = new AbortController();

    async function loadFleets() {
      try {
        const response = await getDriverFleetsRequest({
          signal: controller.signal,
        });

        if (!controller.signal.aborted) {
          setFleets(response?.data ?? []);
        }
      } catch (requestError) {
        if (requestError?.name !== "AbortError") {
          setError(requestError?.message || "Unable to load driver fleets.");
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      }
    }

    void loadFleets();

    return () => {
      controller.abort();
    };
  }, []);

  async function handleSubmit(event) {
    event.preventDefault();

    setError("");
    setRegisteredVehicle(null);
    setIsSubmitting(true);

    try {
      const response = await registerDriverVehicleRequest({
        fleetId,
        registrationNumber: registrationNumber.trim(),
        vin: vin.trim(),
        make: make.trim(),
        model: model.trim(),
        modelYear: Number(modelYear),
        color: color.trim(),
        vehicleType,
        fuelType,
        seatingCapacity: Number(seatingCapacity),
      });

      setRegisteredVehicle(response?.data ?? null);
    } catch (requestError) {
      setError(requestError?.message || "Unable to register vehicle.");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (isLoading) {
    return <div className="page-status">Loading vehicle registration...</div>;
  }

  return (
    <section className="onboarding-card">
      <h1>Register vehicle</h1>

      <p className="muted">
        Register a vehicle for your CONNECT driver account.
      </p>

      {fleets.length === 0 ? (
        <p className="error-message" role="alert">
          No active fleet is available for your driver account.
        </p>
      ) : (
        <form className="onboarding-form" onSubmit={handleSubmit}>
          <label>
            Fleet
            <select
              value={fleetId}
              onChange={(event) => setFleetId(event.target.value)}
              required
            >
              <option value="">Select fleet</option>

              {fleets.map((fleet) => (
                <option key={fleet.id} value={fleet.id}>
                  {fleet.name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Registration number
            <input
              type="text"
              value={registrationNumber}
              onChange={(event) => setRegistrationNumber(event.target.value)}
              required
            />
          </label>

          <label>
            VIN (optional)
            <input
              type="text"
              value={vin}
              onChange={(event) => setVin(event.target.value)}
            />
          </label>

          <label>
            Make
            <input
              type="text"
              value={make}
              onChange={(event) => setMake(event.target.value)}
              required
            />
          </label>

          <label>
            Model
            <input
              type="text"
              value={model}
              onChange={(event) => setModel(event.target.value)}
              required
            />
          </label>

          <label>
            Model year
            <input
              type="number"
              min="1900"
              max={new Date().getUTCFullYear() + 1}
              value={modelYear}
              onChange={(event) => setModelYear(event.target.value)}
              required
            />
          </label>

          <label>
            Color
            <input
              type="text"
              value={color}
              onChange={(event) => setColor(event.target.value)}
            />
          </label>

          <label>
            Vehicle type
            <select
              value={vehicleType}
              onChange={(event) => setVehicleType(event.target.value)}
              required
            >
              <option value="">Select vehicle type</option>

              {VEHICLE_TYPES.map((type) => (
                <option key={type} value={type}>
                  {type}
                </option>
              ))}
            </select>
          </label>

          <label>
            Fuel type
            <select
              value={fuelType}
              onChange={(event) => setFuelType(event.target.value)}
              required
            >
              <option value="">Select fuel type</option>

              {FUEL_TYPES.map((type) => (
                <option key={type} value={type}>
                  {type}
                </option>
              ))}
            </select>
          </label>

          <label>
            Seating capacity
            <input
              type="number"
              min="1"
              value={seatingCapacity}
              onChange={(event) => setSeatingCapacity(event.target.value)}
              required
            />
          </label>

          {error ? (
            <p className="error-message" role="alert">
              {error}
            </p>
          ) : null}

          {registeredVehicle ? (
            <p>
              Vehicle <strong>{registeredVehicle.registration_number}</strong>{" "}
              registered successfully.
            </p>
          ) : null}

          <button type="submit" disabled={isSubmitting}>
            {isSubmitting ? "Registering..." : "Register vehicle"}
          </button>
        </form>
      )}
    </section>
  );
}
