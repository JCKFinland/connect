import { createBrowserRouter, Navigate } from "react-router";

import { DriverLayout } from "../layouts/DriverLayout";
import { DashboardPage } from "../pages/DashboardPage";
import { LoginPage } from "../pages/LoginPage";
import { OnboardingPage } from "../pages/OnboardingPage";
import { VehicleRegistrationPage } from "../pages/VehicleRegistrationPage";
import { DriverRoute } from "./DriverRoute";
import { ProtectedRoute } from "./ProtectedRoute";

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <DriverLayout />,
        children: [
          {
            path: "onboarding",
            element: <OnboardingPage />,
          },
          {
            element: <DriverRoute />,
            children: [
              {
                index: true,
                element: <DashboardPage />,
              },
              {
                path: "vehicles/register",
                element: <VehicleRegistrationPage />,
              },
            ],
          },
        ],
      },
    ],
  },
  {
    path: "*",
    element: <Navigate to="/" replace />,
  },
]);