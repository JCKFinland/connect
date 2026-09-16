import { apiRequest } from "./client";

export function getTripPaymentRequest(
  tripId,
  options = {},
) {
  return apiRequest(
    `/trips/${encodeURIComponent(tripId)}/payment`,
    {
      method: "GET",
      authenticated: true,
      signal: options.signal,
    },
  );
}

export function getPaymentRequest(
  paymentId,
  options = {},
) {
  return apiRequest(
    `/payments/${encodeURIComponent(paymentId)}`,
    {
      method: "GET",
      authenticated: true,
      signal: options.signal,
    },
  );
}

export function initiateSaleTransactionRequest(
  paymentId,
  idempotencyKey,
) {
  return apiRequest(
    `/payments/${encodeURIComponent(paymentId)}/transactions`,
    {
      method: "POST",
      authenticated: true,
      body: {
        provider: "STRIPE",
        idempotency_key: idempotencyKey,
        transaction_type: "SALE",
      },
    },
  );
}

export function executePaymentTransactionRequest(
  paymentId,
  transactionId,
) {
  return apiRequest(
    `/payments/${encodeURIComponent(paymentId)}/transactions/${encodeURIComponent(transactionId)}/execute`,
    {
      method: "POST",
      authenticated: true,
    },
  );
}
