import { useEffect, useState } from "react";

import { ApiError } from "../api/client";
import {
  getDriverRegistrationRequest,
  getRegistrationBranchesRequest,
  getRegistrationCompaniesRequest,
  registerDriverRequest,
} from "../api/driverRegistration";

export function OnboardingPage() {
  const [registration, setRegistration] = useState(null);
  const [companies, setCompanies] = useState([]);
  const [branches, setBranches] = useState([]);

  const [companyId, setCompanyId] = useState("");
  const [branchId, setBranchId] = useState("");
  const [taxiDriverLicenseNumber, setTaxiDriverLicenseNumber] = useState("");
  const [drivingLicenseNumber, setDrivingLicenseNumber] = useState("");
  const [drivingLicenseExpiry, setDrivingLicenseExpiry] = useState("");

  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingBranches, setIsLoadingBranches] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();

    async function loadOnboarding() {
      try {
        const registrationResponse = await getDriverRegistrationRequest({
          signal: controller.signal,
        });

        setRegistration(registrationResponse?.data ?? null);
      } catch (requestError) {
        if (requestError?.name === "AbortError") {
          return;
        }

        if (!(
          requestError instanceof ApiError && requestError.status === 404
        )) {
          setError(
            requestError?.message || "Unable to load driver registration.",
          );
          return;
        }

        try {
          const companiesResponse = await getRegistrationCompaniesRequest({
            signal: controller.signal,
          });

          setCompanies(companiesResponse?.data ?? []);
        } catch (companiesError) {
          if (companiesError?.name !== "AbortError") {
            setError(
              companiesError?.message ||
                "Unable to load registration companies.",
            );
          }
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      }
    }

    void loadOnboarding();

    return () => {
      controller.abort();
    };
  }, []);

  useEffect(() => {
    if (!companyId) {
      return;
    }

    const controller = new AbortController();

    async function loadBranches() {
      setIsLoadingBranches(true);
      setBranchId("");
      setError("");

      try {
        const response = await getRegistrationBranchesRequest(companyId, {
          signal: controller.signal,
        });

        setBranches(response?.data ?? []);
      } catch (requestError) {
        if (requestError?.name !== "AbortError") {
          setBranches([]);
          setError(
            requestError?.message || "Unable to load registration branches.",
          );
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoadingBranches(false);
        }
      }
    }

    void loadBranches();

    return () => {
      controller.abort();
    };
  }, [companyId]);

  async function handleSubmit(event) {
    event.preventDefault();

    setError("");
    setIsSubmitting(true);

    try {
      const response = await registerDriverRequest({
        companyId,
        branchId,
        taxiDriverLicenseNumber: taxiDriverLicenseNumber.trim(),
        drivingLicenseNumber: drivingLicenseNumber.trim(),
        drivingLicenseExpiry: new Date(
          `${drivingLicenseExpiry}T23:59:59`,
        ).toISOString(),
      });

      setRegistration(response?.data ?? null);
    } catch (requestError) {
      setError(requestError?.message || "Unable to submit driver application.");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (isLoading) {
    return <div className="page-status">Loading driver registration...</div>;
  }

  if (registration) {
    return (
      <section className="onboarding-card">
        <h1>Driver registration</h1>
        <p>Your application has been submitted.</p>
        <p>
          Status: <strong>{registration.status}</strong>
        </p>
      </section>
    );
  }

  return (
    <section className="onboarding-card">
      <h1>Become a CONNECT driver</h1>
      <p className="muted">
        Complete your driver application. Your application must be verified
        before driver operations become available.
      </p>

      <form className="onboarding-form" onSubmit={handleSubmit}>
        <label>
          Company
          <select
            value={companyId}
            onChange={(event) => {
              setCompanyId(event.target.value);
              setBranchId("");
              setBranches([]);
            }}
            required
          >
            <option value="">Select company</option>
            {companies.map((company) => (
              <option key={company.id} value={company.id}>
                {company.name}
              </option>
            ))}
          </select>
        </label>

        <label>
          Branch
          <select
            value={branchId}
            onChange={(event) => setBranchId(event.target.value)}
            disabled={!companyId || isLoadingBranches}
            required
          >
            <option value="">
              {isLoadingBranches ? "Loading branches..." : "Select branch"}
            </option>
            {branches.map((branch) => (
              <option key={branch.id} value={branch.id}>
                {branch.name}
              </option>
            ))}
          </select>
        </label>

        <label>
          Taxi driver licence number
          <input
            type="text"
            value={taxiDriverLicenseNumber}
            onChange={(event) => setTaxiDriverLicenseNumber(event.target.value)}
            required
          />
        </label>

        <label>
          Driving licence number
          <input
            type="text"
            value={drivingLicenseNumber}
            onChange={(event) => setDrivingLicenseNumber(event.target.value)}
            required
          />
        </label>

        <label>
          Driving licence expiry
          <input
            type="date"
            value={drivingLicenseExpiry}
            onChange={(event) => setDrivingLicenseExpiry(event.target.value)}
            required
          />
        </label>

        {error ? (
          <p className="error-message" role="alert">
            {error}
          </p>
        ) : null}

        <button type="submit" disabled={isSubmitting || isLoadingBranches}>
          {isSubmitting ? "Submitting..." : "Submit application"}
        </button>
      </form>
    </section>
  );
}
