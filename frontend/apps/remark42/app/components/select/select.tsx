import clsx from 'clsx';
import { h } from 'preact';

import { useTheme } from 'hooks/useTheme';
import { Dropdown } from 'components/dropdown';

import styles from './select.module.css';

type Item = {
  label: string | number;
  value: string | number | undefined;
};

type Props = {
  size?: 'sm' | 'md';
  items: Item[];
  selected?: Item;
  title?: string;
  contentAlign?: 'left' | 'right';
  onChange(value: Item['value']): void;
};

export function Select({ items, selected, size = 'md', title, contentAlign, onChange }: Props) {
  const theme = useTheme();
  const selectedItem = selected ?? items[0];

  return (
    <Dropdown
      title={String(selectedItem.label)}
      buttonTitle={title}
      theme={theme}
      contentAlign={contentAlign}
      titleClass={clsx(styles.trigger, size && styles[size])}
    >
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          role="option"
          aria-selected={item.value === selectedItem.value}
          className={clsx(styles.option, item.value === selectedItem.value && styles.optionSelected)}
          onClick={() => onChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </Dropdown>
  );
}
