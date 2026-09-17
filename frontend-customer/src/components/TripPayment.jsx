import { Elements } from "@stripe/react-stripe-js";
import { useEffect, useState } from "react";

import {
  executePaymentTransactionRequest,
  getPaymentRequest,
  getTripPaymentRequest,
  initiateSaleTransactionRequest,
} from "../api/payments";
import { stripePromise } from "../config/stripe";
import StripePaymentForm from "./StripePaymentForm";

const TERMINAL_STATUSES = new Set([
  "PAID",
  "FAILED",
  "CANCELLED",
]);

const PAYMENT_POLL_INTERVAL_MS = 1000;
const PAYMENT_POLL_ATTEMPTS = 15;

function delay(milliseconds) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, milliseconds);
  });
}

export default function TripPayment({ tripId }) {
  const [payment, setPayment] = useState(null);
  const [clientSecret, setClientSecret] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [isPreparing, setIsPreparing] = useState(false);
  const [isConfirming, setIsConfirming] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();

    async function loadPayment() {
      try {
        const response = await getTripPaymentRequest(
          tripId,
          {
            signal: controller.signal,
          },
        );

        const loadedPayment = response.data;
        setPayment(loadedPayment);

        if (loadedPayment.status === "PENDING") {
          const idempotencyKey =
            `customer-sale-${loadedPayment.id}`;

          const transactionResponse =
            await initiateSaleTransactionRequest(
              loadedPayment.id,
              idempotencyKey,
            );

          const executionResponse =
            await executePaymentTransactionRequest(
              loadedPayment.id,
              transactionResponse.data.id,
            );

          if (!executionResponse.data.client_secret) {
            throw new Error(
              "Stripe client secret was not returned",
            );
          }

          setClientSecret(
            executionResponse.data.client_secret,
          );
        }
      } catch (requestError) {
        if (controller.signal.aborted) {
          return;
        }

        setError(
          requestError.message ??
            "Unable to load payment",
        );
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      }
    }

    void loadPayment();

    return () => controller.abort();
  }, [tripId]);

  async function preparePayment() {
    if (!payment || isPreparing) {
      return;
    }

    setIsPreparing(true);
    setError("");

    try {
      const idempotencyKey =
        `customer-sale-${payment.id}`;

      const transactionResponse =
        await initiateSaleTransactionRequest(
          payment.id,
          idempotencyKey,
        );

      const executionResponse =
        await executePaymentTransactionRequest(
          payment.id,
          transactionResponse.data.id,
        );

      if (!executionResponse.data.client_secret) {
        throw new Error(
          "Stripe client secret was not returned",
        );
      }

      setClientSecret(
        executionResponse.data.client_secret,
      );
    } catch (requestError) {
      setError(
        requestError.message ??
          "Unable to prepare payment",
      );
    } finally {
      setIsPreparing(false);
    }
  }

  async function handleConfirmed() {
    if (!payment || isConfirming) {
      return;
    }

    setIsConfirming(true);
    setError("");

    try {
      for (
        let attempt = 0;
        attempt < PAYMENT_POLL_ATTEMPTS;
        attempt += 1
      ) {
        const response =
          await getPaymentRequest(payment.id);

        const refreshedPayment = response.data;
        setPayment(refreshedPayment);

        if (
          TERMINAL_STATUSES.has(
            refreshedPayment.status,
          )
        ) {
          return;
        }

        await delay(PAYMENT_POLL_INTERVAL_MS);
      }

      setError(
        "Payment is still processing. Its status will update after provider confirmation.",
      );
    } catch (requestError) {
      setError(
        requestError.message ??
          "Unable to refresh payment status",
      );
    } finally {
      setIsConfirming(false);
    }
  }

  if (isLoading) {
    return <p>Loading payment...</p>;
  }

  if (error && !payment) {
    return <p role="alert">{error}</p>;
  }

  if (!payment) {
    return null;
  }

  if (payment.status === "PAID") {
    return (
      <section>
        <h3>Payment</h3>
        <p>Payment completed.</p>
      </section>
    );
  }

  return (
    <section>
      <h3>Payment</h3>

      <p>
        Amount: {payment.amount} {payment.currency}
      </p>

      <p>Status: {payment.status}</p>

      {isConfirming ? (
        <p>
          Waiting for payment confirmation...
        </p>
      ) : null}

      {error ? <p role="alert">{error}</p> : null}

      {!clientSecret ? (
        <button
          type="button"
          onClick={preparePayment}
          disabled={isPreparing}
        >
          {isPreparing
            ? "Preparing payment..."
            : "Pay with card"}
        </button>
      ) : (
        <Elements
          stripe={stripePromise}
          options={{
            clientSecret,
          }}
        >
          <StripePaymentForm
            payment={payment}
            onConfirmed={handleConfirmed}
          />
        </Elements>
      )}
    </section>
  );
}
