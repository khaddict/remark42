import { OAuthProvider, Theme } from 'common/types';
import { capitalizeFirstLetter } from 'utils/capitalize-first-letter';
import { siteId } from 'common/settings';
import { BASE_URL } from 'common/constants.config';

import { OAUTH_DATA } from './oauth.consts';

const oauthReturnLocation = encodeURIComponent(`${window.location.origin}${window.location.pathname}?selfClose`);

export function getOAuthLoginHref(provider: OAuthProvider) {
  return `${BASE_URL}/auth/${provider}/login?from=${oauthReturnLocation}&site=${siteId}`;
}

export function getProviderData(provider: OAuthProvider, theme: Theme) {
  const data = OAUTH_DATA[provider as keyof typeof OAUTH_DATA];

  if (!data) {
    return { name: capitalizeFirstLetter(provider), icon: OAUTH_DATA.custom };
  }

  if (typeof data !== 'string') {
    return {
      name: data.name,
      icon: data.icons[theme],
    };
  }

  return { name: capitalizeFirstLetter(provider), icon: data };
}
