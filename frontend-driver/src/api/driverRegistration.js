import { apiRequest } from "./client";

export function getDriverRegistrationRequest({ signal } = {}) {
  return apiRequest("/drivers/registration", {
    authenticated: true,
    signal,
  });
}

export function getRegistrationCompaniesRequest({ signal } = {}) {
  return apiRequest("/drivers/registration/companies", {
    authenticated: true,
    signal,
  });
}

export function getRegistrationBranchesRequest(companyId, { signal } = {}) {
  return apiRequest(
    `/drivers/registration/companies/${encodeURIComponent(companyId)}/branches`,
    {
      authenticated: true,
      signal,
    },
  );
}

export function registerDriverRequest({
  companyId,
  branchId,
  taxiDriverLicenseNumber,
  drivingLicenseNumber,
  drivingLicenseExpiry,
}) {
  return apiRequest("/drivers/register", {
    method: "POST",
    authenticated: true,
    body: {
      company_id: companyId,
      branch_id: branchId,
      taxi_driver_license_number: taxiDriverLicenseNumber,
      driving_license_number: drivingLicenseNumber,
      driving_license_expiry: drivingLicenseExpiry,
    },
  });
}
