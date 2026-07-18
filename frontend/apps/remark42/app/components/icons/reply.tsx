import { h, JSX } from 'preact';

type Props = Omit<JSX.SVGAttributes<SVGSVGElement>, 'size'> & {
  size?: number | string;
};

export function ReplyIcon({ size = 15, ...props }: Props) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.5"
        d="M6.5 3L2 7.5L6.5 12M2 7.5H9C11.7614 7.5 14 9.73858 14 12.5V13"
      />
    </svg>
  );
}
