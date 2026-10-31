import type { SVGProps } from 'react'

export type IconName =
  | 'logo'
  | 'ask'
  | 'materials'
  | 'logout'
  | 'class'
  | 'shield'
  | 'spark'
  | 'send'
  | 'stop'
  | 'upload'
  | 'file'
  | 'check'
  | 'alert'
  | 'clock'
  | 'quote'
  | 'refresh'
  | 'book'
  | 'arrow'
  | 'user'
  | 'chevron'
  | 'close'

interface IconProps extends Omit<SVGProps<SVGSVGElement>, 'name'> {
  name: IconName
  size?: number
}

export default function Icon({ name, size = 20, ...props }: IconProps) {
  const paths: Record<IconName, React.ReactNode> = {
    logo: (
      <>
        <path d="M5 5.5 12 3l7 2.5v12L12 20l-7-2.5z" />
        <path d="M8 8.2 12 7l4 1.2M8 12l4-1 4 1M8 15.8l4-1 4 1" />
      </>
    ),
    ask: (
      <>
        <path d="M20 11.5a7.5 7.5 0 0 1-8 7.5 9.4 9.4 0 0 1-3.4-.6L4 20l1.5-4.2A7.2 7.2 0 0 1 4 11.5 7.5 7.5 0 0 1 12 4a7.5 7.5 0 0 1 8 7.5Z" />
        <path d="M9.7 9.3a2.4 2.4 0 0 1 4.6.8c0 1.8-2.3 2-2.3 3.3M12 16.2h.01" />
      </>
    ),
    materials: (
      <>
        <path d="M5 3.5h11a3 3 0 0 1 3 3v14H8a3 3 0 0 1-3-3z" />
        <path d="M8 3.5v17M11.5 8h4M11.5 11.5h4" />
      </>
    ),
    logout: (
      <>
        <path d="M10 4H5.5A1.5 1.5 0 0 0 4 5.5v13A1.5 1.5 0 0 0 5.5 20H10M15 8l4 4-4 4M8 12h11" />
      </>
    ),
    class: (
      <>
        <path d="M3.5 9 12 4l8.5 5-8.5 5zM6 11.2V16c3.5 2.7 8.5 2.7 12 0v-4.8M20.5 9v6" />
      </>
    ),
    shield: (
      <>
        <path d="M12 3 20 6v5.5c0 4.8-3.3 8-8 9.5-4.7-1.5-8-4.7-8-9.5V6z" />
        <path d="m8.5 12 2.2 2.2 4.8-5" />
      </>
    ),
    spark: (
      <>
        <path d="m12 3 1.5 5.5L19 10l-5.5 1.5L12 17l-1.5-5.5L5 10l5.5-1.5zM19 16l.7 2.3L22 19l-2.3.7L19 22l-.7-2.3L16 19l2.3-.7z" />
      </>
    ),
    send: (
      <>
        <path d="m21 3-8.2 18-2-7.8L3 11.2zM10.8 13.2 21 3" />
      </>
    ),
    stop: <path d="M7 7h10v10H7z" />,
    upload: (
      <>
        <path d="M12 16V4M7.5 8.5 12 4l4.5 4.5M4 15v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4" />
      </>
    ),
    file: (
      <>
        <path d="M6 3h8l4 4v14H6zM14 3v5h5M9 12h6M9 16h6" />
      </>
    ),
    check: <path d="m5 12.5 4.2 4.2L19.5 6.5" />,
    alert: (
      <>
        <path d="M12 3 2.8 20h18.4zM12 9v4.5M12 17h.01" />
      </>
    ),
    clock: (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M12 7v5l3.5 2" />
      </>
    ),
    quote: (
      <>
        <path d="M5 5h14v14H5zM8 15.5l3-7M13 15.5l3-7" />
      </>
    ),
    refresh: (
      <>
        <path d="M20 7v5h-5M4 17v-5h5" />
        <path d="M6.1 8.2A7 7 0 0 1 18.8 10M5.2 14a7 7 0 0 0 12.7 1.8" />
      </>
    ),
    book: (
      <>
        <path d="M4 5.5A3.5 3.5 0 0 1 7.5 2H11v17H7.5A3.5 3.5 0 0 0 4 22zM20 5.5A3.5 3.5 0 0 0 16.5 2H13v17h3.5A3.5 3.5 0 0 1 20 22z" />
      </>
    ),
    arrow: <path d="M5 12h14M14 7l5 5-5 5" />,
    user: (
      <>
        <circle cx="12" cy="8" r="4" />
        <path d="M4.5 21a7.5 7.5 0 0 1 15 0" />
      </>
    ),
    chevron: <path d="m8 10 4 4 4-4" />,
    close: <path d="M6 6l12 12M18 6 6 18" />,
  }

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {paths[name]}
    </svg>
  )
}
