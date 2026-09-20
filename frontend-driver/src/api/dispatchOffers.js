import { apiRequest } from "./client";

export function getPendingDispatchOffer() {
  return apiRequest("/drivers/dispatch-offers/pending", {
    authenticated: true,
  });
}

export function acceptDispatchOffer(offerId) {
  return apiRequest(`/drivers/dispatch-offers/${offerId}/accept`, {
    method: "POST",
    authenticated: true,
    body: {},
  });
}

export function rejectDispatchOffer(offerId, reason) {
  return apiRequest(`/drivers/dispatch-offers/${offerId}/reject`, {
    method: "POST",
    authenticated: true,
    body: {
      reason,
    },
  });
}
