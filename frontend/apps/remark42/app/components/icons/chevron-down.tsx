import { h, JSX } from 'preact';

type Props = JSX.HTMLAttributes<SVGSVGElement> & { size?: number };

export function ChevronDownIcon({ size = 10, ...props }: Props) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}
