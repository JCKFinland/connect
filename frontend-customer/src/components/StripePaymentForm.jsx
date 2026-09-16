import {
  PaymentElement,
  useElements,
  useStripe,
} from "@stripe/react-stripe-js";
import { useState } from "react";

export default function StripePaymentForm({
  payment,
  onConfirmed,
}) {
  const stripe = useStripe();
  const elements = useElements();

  const [isSubmitting, setIsSubmitting] =
    useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();

    if (!stripe || !elements || isSubmitting) {
      return;
    }

    setIsSubmitting(true);
    setError("");

    const { error: stripeError } =
      await stripe.confirmPayment({
        elements,
        redirect: "if_required",
      });

    if (stripeError) {
      setError(
        stripeError.message ??
          "Payment confirmation failed",
      );
      setIsSubmitting(false);
      return;
    }

    setIsSubmitting(false);
    onConfirmed?.();
  }

  return (
    <form onSubmit={handleSubmit}>
      <PaymentElement />

      {error ? <p role="alert">{error}</p> : null}

      <button
        type="submit"
        disabled={!stripe || !elements || isSubmitting}
      >
        {isSubmitting
          ? "Processing payment..."
          : `Pay ${payment.amount} ${payment.currency}`}
      </button>
    </form>
  );
}
