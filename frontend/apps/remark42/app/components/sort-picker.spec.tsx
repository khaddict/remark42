import '@testing-library/jest-dom';
import { screen, fireEvent, waitFor } from '@testing-library/preact';

import { render } from 'tests/utils';
import * as commentsActions from 'store/comments/actions';
import type { StoreState } from 'store';

import { SortPicker } from './sort-picker';

const defaultState = { comments: {} as StoreState['comments'], hiddenUsers: {} };

describe('<SortPicker />', () => {
  it('should render sort picker with options', () => {
    const { queryAllByText, queryByText } = render(<SortPicker />, defaultState);

    fireEvent.click(screen.getByText('Best'));

    expect(screen.getAllByRole('option')).toHaveLength(4);
    expect(queryAllByText('Best')).toHaveLength(2);
    expect(queryByText('Sort by')).toBeInTheDocument();
  });

  it('should has static class names', () => {
    const { container } = render(<SortPicker />, defaultState);

    expect(container.querySelector('.sort-picker')).toBeInTheDocument();
  });

  it('should render selected element', () => {
    render(<SortPicker />, { comments: { sort: '-time' } as StoreState['comments'] });

    fireEvent.click(screen.getAllByText('Newest')[0]);

    expect(screen.getAllByText('Newest')[1]).toHaveAttribute('aria-selected', 'true');
  });

  it('should change selected store', async () => {
    const nextOption = '+score';
    const updateSorting = jest.spyOn(commentsActions, 'updateSorting');
    render(<SortPicker />, defaultState);

    fireEvent.click(screen.getByText('Best'));
    fireEvent.click(screen.getByText('Worst'));

    await waitFor(() => expect(updateSorting).toHaveBeenCalledWith(nextOption));
  });
});
