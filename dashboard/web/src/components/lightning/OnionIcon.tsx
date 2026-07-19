interface Props {
  className?: string;
}

export const OnionIcon = ({ className }: Props) => (
  <svg
    className={className}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M12 8c3.5 2 5.5 4.2 5.5 7a5.5 5.5 0 0 1-11 0c0-2.8 2-5 5.5-7z" />
    <path d="M12 8c1.4 2 2.2 4.2 2.2 7a7 7 0 0 1-.7 3.2" />
    <path d="M12 8c-1.4 2-2.2 4.2-2.2 7a7 7 0 0 0 .7 3.2" />
    <path d="M12 8V5" />
    <path d="M12 5c-.8-1-2-1.5-3.2-1.3" />
    <path d="M12 5c.8-1 2-1.5 3.2-1.3" />
  </svg>
);
