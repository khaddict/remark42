import '@testing-library/jest-dom';
import { fireEvent, screen } from '@testing-library/preact';

import { render } from 'tests/utils';
import { Select } from './select';

const items = [
  { label: 'None', value: 'none' },
  { label: 'Oldest', value: 'oldest' },
  { label: 'Newest', value: 'newest' },
  { label: 'Best', value: 'best' },
  { label: 'Worst', value: 'worst' },
];

describe('<Select/>', () => {
  it('should render the selected item as the trigger label', () => {
    render(<Select items={items} selected={items[1]} onChange={jest.fn()} />);
    expect(screen.getByText('Oldest')).toBeInTheDocument();
  });

  it('should open the options list when the trigger is clicked', () => {
    render(<Select items={items} selected={items[0]} onChange={jest.fn()} />);

    expect(screen.queryByText('Worst')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('None'));
    expect(screen.getByText('Worst')).toBeInTheDocument();
  });

  it('should call onChange with the clicked item value', () => {
    const onChange = jest.fn();
    render(<Select items={items} selected={items[0]} onChange={onChange} />);

    fireEvent.click(screen.getByText('None'));
    fireEvent.click(screen.getByText('Best'));

    expect(onChange).toHaveBeenCalledWith('best');
  });
});
