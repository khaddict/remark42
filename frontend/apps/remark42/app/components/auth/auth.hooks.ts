import { useState, useMemo } from 'preact/hooks';
import { useIntl } from 'react-intl';

import { errorMessages, RequestError } from 'utils/errorUtils';
import { isObject } from 'utils/is-object';
import { messages } from './auth.messages';

export function useErrorMessage(): [string | null, (e: unknown) => void] {
  const intl = useIntl();
  const [invalidReason, setInvalidReason] = useState<string | number | null>(null);

  return useMemo(() => {
    let errorMessage = invalidReason;

    if (invalidReason !== null && typeof invalidReason === 'string' && messages[invalidReason]) {
      errorMessage = intl.formatMessage(messages[invalidReason]);
    }

    if (invalidReason !== null && errorMessages[invalidReason]) {
      errorMessage = intl.formatMessage(errorMessages[invalidReason]);
    }

    if (typeof errorMessage === 'number') {
      console.error('Wrong error message', errorMessage);
      errorMessage = null;
    }

    function setError(err: unknown): void {
      if (err === null) {
        setInvalidReason(null);
        return;
      }

      if (typeof err === 'string') {
        setInvalidReason(err);
        return;
      }

      const errorReason =
        err instanceof RequestError || isObject(err)
          ? (err as Record<'error', string>).error
          : err instanceof Error
          ? err.message
          : 0;

      setInvalidReason(errorReason);
    }

    return [errorMessage, setError];
  }, [intl, invalidReason]);
}
