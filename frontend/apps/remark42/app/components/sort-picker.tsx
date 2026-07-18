import { h } from 'preact';
import { FormattedMessage, defineMessages, useIntl } from 'react-intl';
import { useMemo } from 'preact/hooks';

import { StoreState, useAppDispatch, useAppSelector } from 'store';
import { Select } from 'components/select';
import { updateSorting } from 'store/comments/actions';
import type { Sorting } from 'common/types';

export function SortPicker() {
  const dispatch = useAppDispatch();
  const intl = useIntl();
  const [items, itemsById] = useMemo(() => {
    const sortOptions = {
      '-score': intl.formatMessage(messages.best),
      '+score': intl.formatMessage(messages.worst),
      '-time': intl.formatMessage(messages.newest),
      '+time': intl.formatMessage(messages.oldest),
    };
    type SortItem = { value: string; label: string };
    const sortItems: SortItem[] = Object.entries(sortOptions).map(([k, v]) => ({ value: k, label: v }));
    const sortById = sortItems.reduce(
      (accum, s) => ({ ...accum, [s.value]: s }),
      {} as Partial<Record<Sorting, SortItem>>
    );

    return [sortItems, sortById];
  }, [intl]);
  const sort = useAppSelector((s: StoreState) => s.comments.sort) || items[0].value;
  const selected = itemsById[sort];

  function handleSortChange(value: Sorting) {
    if (!(value in itemsById)) {
      return;
    }

    dispatch(updateSorting(value));
  }

  return (
    <span className="sort-picker">
      <FormattedMessage id="sort-by" defaultMessage="Sort by" />{' '}
      <Select items={items} selected={selected} contentAlign="right" onChange={handleSortChange} />
    </span>
  );
}

const messages = defineMessages({
  best: {
    id: 'commentsSort.best',
    defaultMessage: 'Best',
  },
  worst: {
    id: 'commentsSort.worst',
    defaultMessage: 'Worst',
  },
  newest: {
    id: 'commentsSort.newest',
    defaultMessage: 'Newest',
  },
  oldest: {
    id: 'commentsSort.oldest',
    defaultMessage: 'Oldest',
  },
});
