import React from 'react'
import { useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import SvgIcon from '@mui/material/SvgIcon'
import CheckCircleOutline from '@mui/icons-material/CheckCircleOutline'

export default function App() {
  let [message, setMessage] = useState('');
  let [success, setSuccess] = useState(false);
  let [sessionId, setSessionId] = useState('');

  useEffect(() => {
    // Check to see if this is a redirect back from Checkout
    const query = new URLSearchParams(window.location.search);

    if (query.get('success')) {
      setSuccess(true);
      setSessionId(query.get('session_id') || '');
    }

    if (query.get('canceled')) {
      setSuccess(false);
      setMessage(
        "Order canceled -- continue to shop around and checkout when you're ready."
      );
    }
  }, [sessionId]);

  if (!success && message === '') {
    return (
      <section>
        <Box className="product" display="flex" alignItems="center">
          <Box className="description" ml={2}>
            <Typography variant="h6">Starter plan</Typography>
            <Typography variant="subtitle1">$20.00 / month</Typography>
          </Box>
        </Box>
        <form action="/create-checkout-session" method="POST">
          {/* Add a hidden field with the lookup_key of your Price */}
          <input type="hidden" name="lookup_key" value="{{PRICE_LOOKUP_KEY}}" />
          <Button
            id="checkout-and-portal-button"
            type="submit"
            variant="contained"
            color="primary"
          >
            Checkout
          </Button>
        </form>
      </section>
    );
  } else if (success && sessionId !== '') {
    return (
      <section>
        <Box className="product" display="flex" alignItems="center">
          <CheckCircleOutline color="primary" fontSize="large" />
          <Box className="description" ml={2}>
            <Typography variant="h6">Subscription to starter plan successful!</Typography>
          </Box>
        </Box>
        <form action="/create-portal-session" method="POST">
          <input
            type="hidden"
            id="session-id"
            name="session_id"
            value={sessionId}
          />
          <Button
            id="checkout-and-portal-button"
            type="submit"
            variant="contained"
            color="primary"
          >
            Manage your billing information
          </Button>
        </form>
      </section>
    );
  } else {
    return (
      <section>
        <Typography variant="body1">{message}</Typography>
      </section>
    );
  }
}
