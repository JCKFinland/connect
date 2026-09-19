import { useEffect, useState } from "react";
import { useNavigate } from "react-router";

import { getFareEstimate } from "../api/fareEstimates";
import { createRideRequest, getServiceCategories } from "../api/rideRequests";

export default function BookRidePage() {
  const navigate = useNavigate();

  const [serviceCategories, setServiceCategories] = useState([]);
  const [isLoadingCategories, setIsLoadingCategories] = useState(true);

  const [pickupAddress, setPickupAddress] = useState("");
  const [pickupLatitude, setPickupLatitude] = useState("");
  const [pickupLongitude, setPickupLongitude] = useState("");

  const [destinationAddress, setDestinationAddress] = useState("");
  const [destinationLatitude, setDestinationLatitude] = useState("");
  const [destinationLongitude, setDestinationLongitude] = useState("");

  const [serviceCategoryId, setServiceCategoryId] = useState("");
  const [passengerCount, setPassengerCount] = useState(1);
  const [notes, setNotes] = useState("");

  const [fareEstimate, setFareEstimate] = useState(null);
  const [isEstimating, setIsEstimating] = useState(false);

  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    async function loadServiceCategories() {
      try {
        const categories = await getServiceCategories({
          signal: controller.signal,
        });

        setServiceCategories(categories);

        if (categories.length > 0) {
          setServiceCategoryId(categories[0].id);
        }
      } catch (requestError) {
        if (requestError?.name !== "AbortError") {
          setError(
            requestError?.message ??
              "Unable to load available service categories",
          );
        }
      } finally {
        setIsLoadingCategories(false);
      }
    }

    loadServiceCategories();

    return () => {
      controller.abort();
    };
  }, []);

  function invalidateFareEstimate() {
    setFareEstimate(null);
  }

  async function handleFareEstimate() {
    setError("");
    setFareEstimate(null);
    setIsEstimating(true);

    try {
      const estimate = await getFareEstimate({
        pickupLatitude: Number(pickupLatitude),
        pickupLongitude: Number(pickupLongitude),
        destinationLatitude: Number(destinationLatitude),
        destinationLongitude: Number(destinationLongitude),
        serviceCategoryId,
      });

      setFareEstimate(estimate);
    } catch (requestError) {
      setError(requestError?.message ?? "Unable to calculate fare estimate");
    } finally {
      setIsEstimating(false);
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();

    if (!fareEstimate) {
      setError("Calculate a fare estimate before booking your ride.");
      return;
    }

    setError("");
    setIsSubmitting(true);

    try {
      const rideRequest = await createRideRequest({
        pickupAddress: pickupAddress.trim(),
        pickupLatitude: Number(pickupLatitude),
        pickupLongitude: Number(pickupLongitude),
        destinationAddress: destinationAddress.trim(),
        destinationLatitude: Number(destinationLatitude),
        destinationLongitude: Number(destinationLongitude),
        serviceCategoryId,
        passengerCount: Number(passengerCount),
        notes: notes.trim(),
      });

      navigate("/", {
        replace: true,
        state: {
          rideRequestCreated: true,
          rideRequestId: rideRequest?.id,
        },
      });
    } catch (requestError) {
      setError(requestError?.message ?? "Unable to book ride");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section>
      <h2>Book a ride</h2>

      <p>Enter your pickup and destination details.</p>

      <form onSubmit={handleSubmit}>
        <fieldset disabled={isSubmitting}>
          <legend>Pickup</legend>

          <div>
            <label htmlFor="pickupAddress">Pickup address</label>

            <input
              id="pickupAddress"
              type="text"
              required
              value={pickupAddress}
              onChange={(event) => setPickupAddress(event.target.value)}
            />
          </div>

          <div>
            <label htmlFor="pickupLatitude">Latitude</label>

            <input
              id="pickupLatitude"
              type="number"
              step="any"
              min="-90"
              max="90"
              required
              value={pickupLatitude}
              onChange={(event) => {
                setPickupLatitude(event.target.value);
                invalidateFareEstimate();
              }}
            />
          </div>

          <div>
            <label htmlFor="pickupLongitude">Longitude</label>

            <input
              id="pickupLongitude"
              type="number"
              step="any"
              min="-180"
              max="180"
              required
              value={pickupLongitude}
              onChange={(event) => {
                setPickupLongitude(event.target.value);
                invalidateFareEstimate();
              }}
            />
          </div>
        </fieldset>

        <fieldset disabled={isSubmitting}>
          <legend>Destination</legend>

          <div>
            <label htmlFor="destinationAddress">Destination address</label>

            <input
              id="destinationAddress"
              type="text"
              required
              value={destinationAddress}
              onChange={(event) => setDestinationAddress(event.target.value)}
            />
          </div>

          <div>
            <label htmlFor="destinationLatitude">Latitude</label>

            <input
              id="destinationLatitude"
              type="number"
              step="any"
              min="-90"
              max="90"
              required
              value={destinationLatitude}
              onChange={(event) => {
                setDestinationLatitude(event.target.value);
                invalidateFareEstimate();
              }}
            />
          </div>

          <div>
            <label htmlFor="destinationLongitude">Longitude</label>

            <input
              id="destinationLongitude"
              type="number"
              step="any"
              min="-180"
              max="180"
              required
              value={destinationLongitude}
              onChange={(event) => {
                setDestinationLongitude(event.target.value);
                invalidateFareEstimate();
              }}
            />
          </div>
        </fieldset>

        <div>
          <label htmlFor="serviceCategory">Service category</label>

          <select
            id="serviceCategory"
            required
            disabled={isLoadingCategories || isSubmitting}
            value={serviceCategoryId}
            onChange={(event) => {
              setServiceCategoryId(event.target.value);
              invalidateFareEstimate();
            }}
          >
            {isLoadingCategories ? <option value="">Loading...</option> : null}

            {!isLoadingCategories && serviceCategories.length === 0 ? (
              <option value="">No service categories available</option>
            ) : null}

            {serviceCategories.map((category) => (
              <option key={category.id} value={category.id}>
                {category.name}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label htmlFor="passengerCount">Passengers</label>

          <input
            id="passengerCount"
            type="number"
            min="1"
            max="20"
            required
            disabled={isSubmitting}
            value={passengerCount}
            onChange={(event) => setPassengerCount(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="notes">Notes</label>

          <textarea
            id="notes"
            disabled={isSubmitting}
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
          />
        </div>

        <div>
          <button
            type="button"
            onClick={handleFareEstimate}
            disabled={
              isEstimating ||
              isSubmitting ||
              isLoadingCategories ||
              !pickupLatitude ||
              !pickupLongitude ||
              !destinationLatitude ||
              !destinationLongitude ||
              !serviceCategoryId
            }
          >
            {isEstimating ? "Calculating estimate..." : "Get fare estimate"}
          </button>
        </div>

        {fareEstimate ? (
          <section aria-live="polite">
            <h3>Fare estimate</h3>

            <p>
              Estimated fare:{" "}
              <strong>
                {fareEstimate.total_amount.toFixed(2)} {fareEstimate.currency}
              </strong>
            </p>

            <p>
              Distance: {(fareEstimate.distance_meters / 1000).toFixed(1)} km
            </p>

            <p>
              Estimated duration:{" "}
              {Math.max(1, Math.round(fareEstimate.duration_seconds / 60))}{" "}
              minutes
            </p>

            <p>
              This is an estimate. The final fare may vary based on the actual
              trip.
            </p>
          </section>
        ) : null}

        {error ? <p role="alert">{error}</p> : null}

        <button
          type="submit"
          disabled={
            isSubmitting ||
            isEstimating ||
            isLoadingCategories ||
            !serviceCategoryId ||
            !fareEstimate
          }
        >
          {isSubmitting ? "Booking ride..." : "Book ride"}
        </button>
      </form>
    </section>
  );
}
