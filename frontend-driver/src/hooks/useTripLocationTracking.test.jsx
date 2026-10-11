/* @vitest-environment jsdom */

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTripLocationTracking } from "./useTripLocationTracking";
import { recordTripLocation } from "../api/trips";

vi.mock("../api/trips", () => ({
  recordTripLocation: vi.fn(),
}));

const activeTrip = {
  id: "test-trip-id",
  status: "IN_PROGRESS",
};

let onPosition;
let onError;
let watchPosition;
let clearWatch;

function emitPosition(accuracy, timestamp = Date.now()) {
  return act(async () => {
    await onPosition({
      timestamp,
      coords: {
        latitude: 60.1,
        longitude: 19.9,
        accuracy,
        altitude: null,
        speed: null,
        heading: null,
      },
    });
  });
}

describe("useTripLocationTracking", () => {
  beforeEach(() => {
    onPosition = undefined;
    onError = undefined;

    watchPosition = vi.fn((success, error) => {
      onPosition = success;
      onError = error;
      return 123;
    });

    clearWatch = vi.fn();

    Object.defineProperty(navigator, "geolocation", {
      configurable: true,
      value: {
        watchPosition,
        clearWatch,
      },
    });

    vi.mocked(recordTripLocation).mockReset();
    vi.mocked(recordTripLocation).mockResolvedValue({ data: {} });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete navigator.geolocation;
  });

  it("shows a warning and rejects a 120-meter reading", async () => {
    const { result } = renderHook(() =>
      useTripLocationTracking(activeTrip),
    );

    expect(watchPosition).toHaveBeenCalledOnce();

    await emitPosition(120);

    expect(result.current.locationError).toContain(
      "Location accuracy is 120 m",
    );
    expect(result.current.locationError).toContain(
      "50 m or better",
    );
    expect(recordTripLocation).not.toHaveBeenCalled();
  });

  it("accepts a reading at the 50-meter boundary", async () => {
    const { result } = renderHook(() =>
      useTripLocationTracking(activeTrip),
    );

    await emitPosition(50);

    await waitFor(() => {
      expect(recordTripLocation).toHaveBeenCalledOnce();
    });

    expect(recordTripLocation).toHaveBeenCalledWith(
      activeTrip.id,
      expect.objectContaining({
        accuracy_meters: 50,
      }),
    );

    expect(result.current.locationError).toBe("");
  });

  it("clears an accuracy warning after successful recording", async () => {
    const { result } = renderHook(() =>
      useTripLocationTracking(activeTrip),
    );

    const timestamp = Date.now();

    await emitPosition(120, timestamp);

    expect(result.current.locationError).toContain(
      "Location accuracy is 120 m",
    );

    await emitPosition(25, timestamp + 6000);

    await waitFor(() => {
      expect(recordTripLocation).toHaveBeenCalledOnce();
      expect(result.current.locationError).toBe("");
    });
  });

  it("does not track a trip that is not in progress", () => {
    const { result } = renderHook(() =>
      useTripLocationTracking({
        ...activeTrip,
        status: "COMPLETED",
      }),
    );

    expect(watchPosition).not.toHaveBeenCalled();
    expect(recordTripLocation).not.toHaveBeenCalled();
    expect(result.current.locationError).toBe("");
  });

  it("stops tracking when the trip leaves IN_PROGRESS", () => {
    const { rerender } = renderHook(
      ({ trip }) => useTripLocationTracking(trip),
      {
        initialProps: { trip: activeTrip },
      },
    );

    expect(watchPosition).toHaveBeenCalledOnce();

    rerender({
      trip: { ...activeTrip, status: "COMPLETED" },
    });

    expect(clearWatch).toHaveBeenCalledWith(123);
    expect(recordTripLocation).not.toHaveBeenCalled();
  });

  it("reports geolocation errors", () => {
    const { result } = renderHook(() =>
      useTripLocationTracking(activeTrip),
    );

    act(() => {
      onError({
        code: 1,
        message: "User denied Geolocation",
      });
    });

    expect(result.current.locationError).toBe(
      "User denied Geolocation",
    );
  });
});
