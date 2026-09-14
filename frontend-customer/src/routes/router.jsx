import {
  createBrowserRouter,
} from 'react-router'

import CustomerLayout from '../layouts/CustomerLayout'
import HomePage from '../pages/HomePage'
import LoginPage from '../pages/LoginPage'
import RegisterPage from '../pages/RegisterPage'
import TripPage from '../pages/TripPage'

export const router = createBrowserRouter([
  {
    path: '/',
    Component: CustomerLayout,
    children: [
      {
        index: true,
        Component: HomePage,
      },
      {
        path: 'login',
        Component: LoginPage,
      },
      {
        path: 'register',
        Component: RegisterPage,
      },
      {
        path: 'trips/:tripId',
        Component: TripPage,
      },
    ],
  },
])
