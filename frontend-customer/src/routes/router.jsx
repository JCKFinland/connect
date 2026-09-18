import { createBrowserRouter } from "react-router";

import CustomerLayout from "../layouts/CustomerLayout";
import BookRidePage from "../pages/BookRidePage";
import HomePage from "../pages/HomePage";
import LoginPage from "../pages/LoginPage";
import RegisterPage from "../pages/RegisterPage";
import TripPage from "../pages/TripPage";
import ProtectedRoute from "./ProtectedRoute";

export const router = createBrowserRouter([
  {
    path: "/",
    Component: CustomerLayout,
    children: [
      {
        index: true,
        Component: HomePage,
      },
      {
        path: "login",
        Component: LoginPage,
      },
      {
        path: "register",
        Component: RegisterPage,
      },
      {
        Component: ProtectedRoute,
        children: [
          {
            path: "book",
            Component: BookRidePage,
          },
          {
            path: "trips/:tripId",
            Component: TripPage,
          },
        ],
      },
    ],
  },
]);
