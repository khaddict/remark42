import clsx from 'clsx';
import { h } from 'preact';
import { useDispatch } from 'react-redux';
import { useIntl } from 'react-intl';

import { setUser } from 'store/user/actions';
import { GitHubIcon } from 'components/icons/github';
import { useTheme } from 'hooks/useTheme';

import buttonStyles from './components/button.module.css';
import { getOAuthLoginHref, getProviderData } from './components/oauth.utils';
import { messages } from './auth.messages';
import { useErrorMessage } from './auth.hooks';
import { getProviders } from './auth.utils';
import { oauthSignin } from './auth.api';

import styles from './auth.module.css';

export function Auth() {
  const intl = useIntl();
  const theme = useTheme();
  const dispatch = useDispatch();
  const oauthProviders = getProviders();
  const [errorMessage, setError] = useErrorMessage();

  async function handleOauthClick(evt: preact.JSX.TargetedMouseEvent<HTMLAnchorElement>) {
    evt.preventDefault();

    try {
      const user = await oauthSignin(evt.currentTarget.href);

      if (user === null) {
        return;
      }
      dispatch(setUser(user));
    } catch (e) {
      setError(e);
    }
  }

  if (oauthProviders.length === 0) {
    return (
      <div className={clsx('auth', styles.root)}>
        <div className={clsx('auth-error', styles.error)}>{intl.formatMessage(messages.noProviders)}</div>
      </div>
    );
  }

  const provider = oauthProviders[0];
  const { name, icon } = getProviderData(provider, theme);

  return (
    <div className={clsx('auth', styles.root)}>
      {errorMessage && <div className={clsx('auth-error', styles.error)}>{errorMessage}</div>}
      <a
        className={clsx('auth-button', buttonStyles.button)}
        href={getOAuthLoginHref(provider)}
        data-provider-name={name}
        onClick={handleOauthClick}
      >
        {provider === 'github' ? (
          <GitHubIcon width={16} height={16} aria-hidden={true} />
        ) : (
          <img src={icon} width="16" height="16" alt="" aria-hidden={true} />
        )}
        {intl.formatMessage(messages.signin)}
      </a>
    </div>
  );
}
