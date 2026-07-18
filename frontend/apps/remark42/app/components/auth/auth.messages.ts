import { defineMessages } from 'react-intl';

export const messages = defineMessages<string>({
  signin: {
    id: 'auth.signin',
    defaultMessage: 'sign in',
  },
  openProfile: {
    id: 'auth.open-profile',
    defaultMessage: 'Open My Profile',
  },
  signout: {
    id: 'auth.signout',
    defaultMessage: 'Sign Out',
  },
  loading: {
    id: 'auth.loading',
    defaultMessage: 'Loading...',
  },
  noProviders: {
    id: 'auth.no-providers',
    defaultMessage: 'No providers available. Maybe you forgot to enable them?',
  },
});
