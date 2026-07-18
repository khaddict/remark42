import { h, JSX } from 'preact';

type Props = Omit<JSX.SVGAttributes<SVGSVGElement>, 'size'> & {
  size?: number | string;
};

export function ClockIcon({ size = 14, ...props }: Props) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <circle cx="8" cy="8" r="6.25" stroke="currentColor" stroke-width="1.5" />
      <path stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 4.5V8l2.5 1.5" />
    </svg>
  );
}
