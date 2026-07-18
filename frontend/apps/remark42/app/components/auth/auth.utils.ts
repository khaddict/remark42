import { StaticStore } from 'common/static-store';
import type { OAuthProvider } from 'common/types';

import { setItem, getItem } from 'common/local-storage';
import { LS_EMAIL_KEY } from 'common/constants';

export function getProviders(): OAuthProvider[] {
  return StaticStore.config.auth_providers;
}

export function persistEmail(email: string) {
  setItem(LS_EMAIL_KEY, email);
}

export function getPersistedEmail() {
  return getItem(LS_EMAIL_KEY) || '';
}

export function resetPersistedEmail() {
  setItem(LS_EMAIL_KEY, '');
}
