import '@testing-library/jest-dom';

import { fireEvent, waitFor, screen } from '@testing-library/preact';
import { render } from 'tests/utils';

import { OAuthProvider, User } from 'common/types';
import { StaticStore } from 'common/static-store';
import { BASE_URL } from 'common/constants.config';
import * as userActions from 'store/user/actions';

import { Auth } from './auth';
import * as api from './auth.api';

window.open = jest.fn();

describe('<Auth/>', () => {
  let defaultProviders = StaticStore.config.auth_providers;

  afterAll(() => {
    StaticStore.config.auth_providers = defaultProviders;
  });

  it('should render a github sign-in link', () => {
    StaticStore.config.auth_providers = ['github'];

    render(<Auth />);

    expect(screen.getByText('sign in')).toHaveClass('auth-button');
    expect(screen.getByText('sign in').closest('a')).toHaveAttribute('data-provider-name', 'GitHub');
  });

  it('should show error when no auth providers are configured', () => {
    StaticStore.config.auth_providers = [];

    const { container } = render(<Auth />);

    expect(container.querySelector('.auth-error')).toBeInTheDocument();
    expect(screen.queryByText('sign in')).not.toBeInTheDocument();
  });

  it('should not set user if unauthorized', async () => {
    StaticStore.config.auth_providers = ['github'];

    const setUser = jest.spyOn(userActions, 'setUser').mockImplementation(jest.fn());
    const oauthSignin = jest.spyOn(api, 'oauthSignin').mockImplementation(async () => null);

    render(<Auth />);
    fireEvent.click(screen.getByText('sign in'));
    await waitFor(() =>
      expect(oauthSignin).toBeCalledWith(
        `${BASE_URL}/auth/github/login?from=http%3A%2F%2Flocalhost%2F%3FselfClose&site=remark`
      )
    );
    expect(setUser).toBeCalledTimes(0);
    expect(screen.getByText('sign in')).toBeInTheDocument();
  });

  it('should set user if authorized', async () => {
    StaticStore.config.auth_providers = ['github'];

    const user = { name: 'UserName1' } as User;
    const setUser = jest.spyOn(userActions, 'setUser').mockImplementation(jest.fn());
    const oauthSignin = jest.spyOn(api, 'oauthSignin').mockImplementation(async () => user);

    render(<Auth />);

    fireEvent.click(screen.getByText('sign in'));

    await waitFor(() =>
      expect(oauthSignin).toBeCalledWith(
        `${BASE_URL}/auth/github/login?from=http%3A%2F%2Flocalhost%2F%3FselfClose&site=remark`
      )
    );
    expect(setUser).toBeCalledWith(user);
  });

  it('should use whatever single provider name the backend reports', async () => {
    StaticStore.config.auth_providers = ['customoidc'] as OAuthProvider[];

    const oauthSignin = jest.spyOn(api, 'oauthSignin').mockImplementation(async () => null);

    render(<Auth />);

    fireEvent.click(screen.getByText('sign in'));

    await waitFor(() =>
      expect(oauthSignin).toBeCalledWith(
        `${BASE_URL}/auth/customoidc/login?from=http%3A%2F%2Flocalhost%2F%3FselfClose&site=remark`
      )
    );
  });
});
