import { useEffect, useState } from "react";
import { useNavigate } from "react-router";

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

  async function handleSubmit(event) {
    event.preventDefault();

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
        <fieldset>
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
              onChange={(event) => setPickupLatitude(event.target.value)}
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
              onChange={(event) => setPickupLongitude(event.target.value)}
            />
          </div>
        </fieldset>

        <fieldset>
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
              onChange={(event) => setDestinationLatitude(event.target.value)}
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
              onChange={(event) => setDestinationLongitude(event.target.value)}
            />
          </div>
        </fieldset>

        <div>
          <label htmlFor="serviceCategory">Service category</label>

          <select
            id="serviceCategory"
            required
            disabled={isLoadingCategories}
            value={serviceCategoryId}
            onChange={(event) => setServiceCategoryId(event.target.value)}
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
            value={passengerCount}
            onChange={(event) => setPassengerCount(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="notes">Notes</label>

          <textarea
            id="notes"
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
          />
        </div>

        {error ? <p role="alert">{error}</p> : null}

        <button
          type="submit"
          disabled={isSubmitting || isLoadingCategories || !serviceCategoryId}
        >
          {isSubmitting ? "Booking ride..." : "Book ride"}
        </button>
      </form>
    </section>
  );
}
